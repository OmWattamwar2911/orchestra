package etcdstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	clientv3 "go.etcd.io/etcd/client/v3"
	pb "orchestra/api/proto/orchestra/v1"
)

var (
	ErrFencingTokenMismatch = errors.New("fencing check failed: node is no longer the active leader")
	ErrCASConflict          = errors.New("optimistic concurrency conflict: task mod revision changed")
	ErrWorkflowNotFound     = errors.New("workflow not found in store")
	ErrTaskNotFound         = errors.New("task not found in store")
)

// Fencer is the interface implemented by election.Candidate to execute leader-fenced etcd transactions.
type Fencer interface {
	Fenced(ops ...clientv3.Op) (*clientv3.TxnResponse, error)
	FencedContext(ctx context.Context, extraCmps []clientv3.Cmp, ops ...clientv3.Op) (*clientv3.TxnResponse, error)
}

// Store provides operations over etcd with leader fencing and atomic compare-and-swap semantics.
type Store struct {
	client *clientv3.Client
}

// NewStore initializes a new Store instance backed by etcd.
func NewStore(client *clientv3.Client) *Store {
	return &Store{client: client}
}

// PutWorkflow stores or updates workflow metadata. Every mutating leader write is routed
// through the Fencer helper to guarantee zombie ex-leaders cannot corrupt state.
func (s *Store) PutWorkflow(ctx context.Context, fencer Fencer, wf *pb.Workflow) error {
	if wf == nil || wf.GetSpec() == nil || wf.GetSpec().GetId() == "" {
		return errors.New("invalid workflow: nil or empty ID")
	}

	data, err := json.Marshal(wf)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	key := WorkflowKey(wf.GetSpec().GetId())
	op := clientv3.OpPut(key, string(data))

	if fencer != nil {
		// Invariant protected: Every leader write is conditioned on holding the valid election key CreateRevision
		resp, err := fencer.FencedContext(ctx, nil, op)
		if err != nil {
			return fmt.Errorf("fenced put workflow failed: %w", err)
		}
		if !resp.Succeeded {
			return ErrFencingTokenMismatch
		}
		return nil
	}

	_, err = s.client.Put(ctx, key, string(data))
	return err
}

// GetWorkflow retrieves a workflow by ID.
func (s *Store) GetWorkflow(ctx context.Context, workflowID string) (*pb.Workflow, error) {
	key := WorkflowKey(workflowID)
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow %s: %w", workflowID, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, ErrWorkflowNotFound
	}

	var wf pb.Workflow
	if err := json.Unmarshal(resp.Kvs[0].Value, &wf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}

	return &wf, nil
}

// ListWorkflows retrieves all workflows currently stored.
func (s *Store) ListWorkflows(ctx context.Context) ([]*pb.Workflow, error) {
	resp, err := s.client.Get(ctx, PrefixWorkflows, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	workflows := make([]*pb.Workflow, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var wf pb.Workflow
		if err := json.Unmarshal(kv.Value, &wf); err != nil {
			continue // skip corrupted entry
		}
		workflows = append(workflows, &wf)
	}
	return workflows, nil
}

// PutTask initializes or writes task state to etcd through the leader Fencer.
func (s *Store) PutTask(ctx context.Context, fencer Fencer, task *pb.Task) (int64, error) {
	if task == nil || task.GetSpec() == nil || task.GetSpec().GetId() == "" {
		return 0, errors.New("invalid task: nil or empty ID")
	}

	workflowID := task.GetStatus().GetWorkflowId()
	taskID := task.GetSpec().GetId()
	key := TaskKey(workflowID, taskID)

	data, err := json.Marshal(task)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal task: %w", err)
	}

	op := clientv3.OpPut(key, string(data))

	if fencer != nil {
		// Invariant protected: Leader fencing ensures only the current elected leader can initialize or modify task state
		resp, err := fencer.FencedContext(ctx, nil, op)
		if err != nil {
			return 0, fmt.Errorf("fenced put task failed: %w", err)
		}
		if !resp.Succeeded {
			return 0, ErrFencingTokenMismatch
		}
		return resp.Header.Revision, nil
	}

	resp, err := s.client.Put(ctx, key, string(data))
	if err != nil {
		return 0, err
	}
	return resp.Header.Revision, nil
}

// GetTask retrieves a single task by workflow ID and task ID.
func (s *Store) GetTask(ctx context.Context, workflowID, taskID string) (*pb.Task, int64, error) {
	key := TaskKey(workflowID, taskID)
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get task %s: %w", key, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, 0, ErrTaskNotFound
	}

	var task pb.Task
	if err := json.Unmarshal(resp.Kvs[0].Value, &task); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal task: %w", err)
	}

	modRev := resp.Kvs[0].ModRevision
	return &task, modRev, nil
}

// ListTasksForWorkflow retrieves all tasks associated with a workflow.
func (s *Store) ListTasksForWorkflow(ctx context.Context, workflowID string) ([]*pb.Task, error) {
	prefix := WorkflowTasksPrefix(workflowID)
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks for workflow %s: %w", workflowID, err)
	}

	tasks := make([]*pb.Task, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var t pb.Task
		if err := json.Unmarshal(kv.Value, &t); err != nil {
			continue
		}
		if t.Status != nil {
			t.Status.ModRevision = kv.ModRevision
		}
		tasks = append(tasks, &t)
	}
	return tasks, nil
}

// CommitTaskResult commits a task completion, failure, or timeout using an atomic compare-and-swap transaction.
// Delivery semantics: At-least-once execution with exactly-one committed result per task.
func (s *Store) CommitTaskResult(ctx context.Context, fencer Fencer, workflowID, taskID string, expectedModRev int64, updatedStatus *pb.TaskStatus) (int64, error) {
	key := TaskKey(workflowID, taskID)

	// Fetch current task spec so we preserve the spec when updating status
	currentTask, currentModRev, err := s.GetTask(ctx, workflowID, taskID)
	if err != nil {
		return 0, err
	}

	if expectedModRev > 0 && currentModRev != expectedModRev {
		return 0, ErrCASConflict
	}

	currentTask.Status = updatedStatus
	data, err := json.Marshal(currentTask)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal updated task: %w", err)
	}

	// Invariant protected: Fencing condition ensures caller is the active leader, and ModRevision check ensures
	// at-least-once task execution yields exactly-one committed result, rejecting duplicate or stale attempt reports.
	casCmp := clientv3.Compare(clientv3.ModRevision(key), "=", currentModRev)
	putOp := clientv3.OpPut(key, string(data))

	if fencer != nil {
		resp, err := fencer.FencedContext(ctx, []clientv3.Cmp{casCmp}, putOp)
		if err != nil {
			return 0, err
		}
		if !resp.Succeeded {
			return 0, ErrCASConflict
		}
		return resp.Header.Revision, nil
	}

	txn := s.client.Txn(ctx).If(casCmp).Then(putOp)
	resp, err := txn.Commit()
	if err != nil {
		return 0, fmt.Errorf("txn commit failed: %w", err)
	}
	if !resp.Succeeded {
		return 0, ErrCASConflict
	}

	return resp.Header.Revision, nil
}

// RegisterWorkerLease binds worker metadata to an etcd lease for heartbeat liveness detection.
func (s *Store) RegisterWorkerLease(ctx context.Context, workerID string, metadata []byte, leaseID clientv3.LeaseID) error {
	key := WorkerKey(workerID)
	_, err := s.client.Put(ctx, key, string(metadata), clientv3.WithLease(leaseID))
	return err
}

// ListActiveWorkers returns all worker registrations currently backed by active leases.
func (s *Store) ListActiveWorkers(ctx context.Context) (map[string][]byte, error) {
	resp, err := s.client.Get(ctx, PrefixWorkers, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list workers: %w", err)
	}

	workers := make(map[string][]byte, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		workers[string(kv.Key)] = kv.Value
	}
	return workers, nil
}
