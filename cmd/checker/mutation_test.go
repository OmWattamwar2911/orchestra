package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/evidence"
	"orchestra/internal/store"
)

// Helper to construct a valid synthetic run directory with diamond DAG
func createValidSyntheticRun(t *testing.T, tmpDir string) {
	wfID := "wf-synthetic-1"

	// 1. Workflows dump
	taskFetch := &pb.Task{
		Spec:   &pb.TaskSpec{Id: "fetch", Type: "echo"},
		Status: &pb.TaskStatus{State: pb.TaskState_TASK_STATE_COMPLETED, CurrentAttempt: 1},
	}
	taskProcess := &pb.Task{
		Spec:   &pb.TaskSpec{Id: "process", Type: "echo", Dependencies: []string{"fetch"}},
		Status: &pb.TaskStatus{State: pb.TaskState_TASK_STATE_COMPLETED, CurrentAttempt: 1},
	}

	wfDump := []WorkflowDumpItem{
		{
			Workflow: &pb.Workflow{
				Spec: &pb.WorkflowSpec{
					Id:            wfID,
					FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST,
					Tasks: []*pb.TaskSpec{
						taskFetch.Spec,
						taskProcess.Spec,
					},
				},
				Status: &pb.WorkflowStatus{
					Id:    wfID,
					State: pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED,
				},
			},
			Tasks: []*pb.Task{taskFetch, taskProcess},
		},
	}
	wfData, _ := json.Marshal(wfDump)
	_ = os.WriteFile(filepath.Join(tmpDir, "workflows.json"), wfData, 0644)

	// 2. Events log: valid order, monotonic leader term 1
	events := []store.StateTransitionEvent{
		{SequenceNumber: 1, Timestamp: 1000, WorkflowID: wfID, FromState: "WORKFLOW_STATE_UNSPECIFIED", ToState: "WORKFLOW_STATE_RUNNING", LeaderTerm: 1},
		{SequenceNumber: 2, Timestamp: 1050, WorkflowID: wfID, TaskID: "fetch", AttemptID: 1, FromState: "TASK_STATE_READY", ToState: "TASK_STATE_ASSIGNED", LeaderTerm: 1},
		{SequenceNumber: 3, Timestamp: 1100, WorkflowID: wfID, TaskID: "fetch", AttemptID: 1, FromState: "TASK_STATE_ASSIGNED", ToState: "TASK_STATE_COMPLETED", LeaderTerm: 1},
		{SequenceNumber: 4, Timestamp: 1150, WorkflowID: wfID, TaskID: "process", AttemptID: 1, FromState: "TASK_STATE_READY", ToState: "TASK_STATE_ASSIGNED", LeaderTerm: 1},
		{SequenceNumber: 5, Timestamp: 1200, WorkflowID: wfID, TaskID: "process", AttemptID: 1, FromState: "TASK_STATE_ASSIGNED", ToState: "TASK_STATE_COMPLETED", LeaderTerm: 1},
		{SequenceNumber: 6, Timestamp: 1250, WorkflowID: wfID, FromState: "WORKFLOW_STATE_RUNNING", ToState: "WORKFLOW_STATE_SUCCEEDED", LeaderTerm: 1},
	}
	writeEvents(t, tmpDir, events)

	// 3. Evidence log
	evidenceLogs := []evidence.EventRecord{
		{TimestampNs: 1050 * 1e6, WorkflowID: wfID, TaskID: "fetch", AttemptID: 1, Event: evidence.EventStart},
		{TimestampNs: 1100 * 1e6, WorkflowID: wfID, TaskID: "fetch", AttemptID: 1, Event: evidence.EventFinish, Status: "SUCCESS"},
		{TimestampNs: 1150 * 1e6, WorkflowID: wfID, TaskID: "process", AttemptID: 1, Event: evidence.EventStart},
		{TimestampNs: 1200 * 1e6, WorkflowID: wfID, TaskID: "process", AttemptID: 1, Event: evidence.EventFinish, Status: "SUCCESS"},
	}
	writeEvidence(t, tmpDir, evidenceLogs)
}

func writeEvents(t *testing.T, dir string, events []store.StateTransitionEvent) {
	f, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatalf("failed to create events.jsonl: %v", err)
	}
	defer f.Close()
	for _, ev := range events {
		b, _ := json.Marshal(ev)
		_, _ = f.Write(append(b, '\n'))
	}
}

func writeEvidence(t *testing.T, dir string, records []evidence.EventRecord) {
	f, err := os.Create(filepath.Join(dir, "evidence.jsonl"))
	if err != nil {
		t.Fatalf("failed to create evidence.jsonl: %v", err)
	}
	defer f.Close()
	for _, r := range records {
		b, _ := json.Marshal(r)
		_, _ = f.Write(append(b, '\n'))
	}
}

// Baseline: uncorrupted synthetic run passes all invariants
func TestMutation_BaselineValid(t *testing.T) {
	tmpDir := t.TempDir()
	createValidSyntheticRun(t, tmpDir)

	report, err := VerifyRunDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.AllPassed {
		t.Fatalf("expected baseline run to pass, got violations: %v", report.Violations)
	}
}

// Mutation 1: Dependency Order Violation
// Task "process" starts before its dependency "fetch" finishes
func TestMutation_DependencyOrderViolation(t *testing.T) {
	tmpDir := t.TempDir()
	createValidSyntheticRun(t, tmpDir)

	// Corrupt events: process starts at t=1020 while fetch completes at t=1100
	wfID := "wf-synthetic-1"
	corruptedEvents := []store.StateTransitionEvent{
		{SequenceNumber: 1, Timestamp: 1000, WorkflowID: wfID, FromState: "WORKFLOW_STATE_UNSPECIFIED", ToState: "WORKFLOW_STATE_RUNNING", LeaderTerm: 1},
		{SequenceNumber: 2, Timestamp: 1020, WorkflowID: wfID, TaskID: "process", AttemptID: 1, FromState: "TASK_STATE_READY", ToState: "TASK_STATE_ASSIGNED", LeaderTerm: 1},
		{SequenceNumber: 3, Timestamp: 1050, WorkflowID: wfID, TaskID: "fetch", AttemptID: 1, FromState: "TASK_STATE_READY", ToState: "TASK_STATE_ASSIGNED", LeaderTerm: 1},
		{SequenceNumber: 4, Timestamp: 1100, WorkflowID: wfID, TaskID: "fetch", AttemptID: 1, FromState: "TASK_STATE_ASSIGNED", ToState: "TASK_STATE_COMPLETED", LeaderTerm: 1},
		{SequenceNumber: 5, Timestamp: 1200, WorkflowID: wfID, TaskID: "process", AttemptID: 1, FromState: "TASK_STATE_ASSIGNED", ToState: "TASK_STATE_COMPLETED", LeaderTerm: 1},
		{SequenceNumber: 6, Timestamp: 1250, WorkflowID: wfID, FromState: "WORKFLOW_STATE_RUNNING", ToState: "WORKFLOW_STATE_SUCCEEDED", LeaderTerm: 1},
	}
	writeEvents(t, tmpDir, corruptedEvents)

	report, err := VerifyRunDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.AllPassed {
		t.Fatal("expected failure on dependency order violation, but checker passed!")
	}
	if report.DependencyViolations == 0 {
		t.Fatalf("expected DependencyViolations > 0, got %d", report.DependencyViolations)
	}
}

// Mutation 2: Single Commit Violation
// Task "fetch" has TWO committed COMPLETED results (e.g. split-brain / duplicate commit)
func TestMutation_SingleCommitViolation(t *testing.T) {
	tmpDir := t.TempDir()
	createValidSyntheticRun(t, tmpDir)

	wfID := "wf-synthetic-1"
	corruptedEvents := []store.StateTransitionEvent{
		{SequenceNumber: 1, Timestamp: 1000, WorkflowID: wfID, FromState: "WORKFLOW_STATE_UNSPECIFIED", ToState: "WORKFLOW_STATE_RUNNING", LeaderTerm: 1},
		{SequenceNumber: 2, Timestamp: 1050, WorkflowID: wfID, TaskID: "fetch", AttemptID: 1, FromState: "TASK_STATE_READY", ToState: "TASK_STATE_ASSIGNED", LeaderTerm: 1},
		{SequenceNumber: 3, Timestamp: 1100, WorkflowID: wfID, TaskID: "fetch", AttemptID: 1, FromState: "TASK_STATE_ASSIGNED", ToState: "TASK_STATE_COMPLETED", LeaderTerm: 1},
		// Second committed COMPLETED result for the same task!
		{SequenceNumber: 4, Timestamp: 1120, WorkflowID: wfID, TaskID: "fetch", AttemptID: 2, FromState: "TASK_STATE_ASSIGNED", ToState: "TASK_STATE_COMPLETED", LeaderTerm: 1},
		{SequenceNumber: 5, Timestamp: 1150, WorkflowID: wfID, TaskID: "process", AttemptID: 1, FromState: "TASK_STATE_READY", ToState: "TASK_STATE_ASSIGNED", LeaderTerm: 1},
		{SequenceNumber: 6, Timestamp: 1200, WorkflowID: wfID, TaskID: "process", AttemptID: 1, FromState: "TASK_STATE_ASSIGNED", ToState: "TASK_STATE_COMPLETED", LeaderTerm: 1},
		{SequenceNumber: 7, Timestamp: 1250, WorkflowID: wfID, FromState: "WORKFLOW_STATE_RUNNING", ToState: "WORKFLOW_STATE_SUCCEEDED", LeaderTerm: 1},
	}
	writeEvents(t, tmpDir, corruptedEvents)

	report, err := VerifyRunDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.AllPassed {
		t.Fatal("expected failure on duplicate commit, but checker passed!")
	}
	if report.SingleCommitViolations == 0 {
		t.Fatalf("expected SingleCommitViolations > 0, got %d", report.SingleCommitViolations)
	}
}

// Mutation 3: No Lost Tasks Violation
// Task "process" left in non-terminal TASK_STATE_RUNNING
func TestMutation_NoLostTasksViolation(t *testing.T) {
	tmpDir := t.TempDir()
	createValidSyntheticRun(t, tmpDir)

	wfID := "wf-synthetic-1"
	taskFetch := &pb.Task{
		Spec:   &pb.TaskSpec{Id: "fetch", Type: "echo"},
		Status: &pb.TaskStatus{State: pb.TaskState_TASK_STATE_COMPLETED, CurrentAttempt: 1},
	}
	// Lost task left in RUNNING state
	taskProcess := &pb.Task{
		Spec:   &pb.TaskSpec{Id: "process", Type: "echo", Dependencies: []string{"fetch"}},
		Status: &pb.TaskStatus{State: pb.TaskState_TASK_STATE_RUNNING, CurrentAttempt: 1},
	}

	wfDump := []WorkflowDumpItem{
		{
			Workflow: &pb.Workflow{
				Spec: &pb.WorkflowSpec{Id: wfID, FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST, Tasks: []*pb.TaskSpec{taskFetch.Spec, taskProcess.Spec}},
				Status: &pb.WorkflowStatus{Id: wfID, State: pb.WorkflowState_WORKFLOW_STATE_FAILED},
			},
			Tasks: []*pb.Task{taskFetch, taskProcess},
		},
	}
	wfData, _ := json.Marshal(wfDump)
	_ = os.WriteFile(filepath.Join(tmpDir, "workflows.json"), wfData, 0644)

	report, err := VerifyRunDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.AllPassed {
		t.Fatal("expected failure on non-terminal task, but checker passed!")
	}
	if report.NoLostTasksViolations == 0 {
		t.Fatalf("expected NoLostTasksViolations > 0, got %d", report.NoLostTasksViolations)
	}
}

// Mutation 4: Workflow Termination Violation
// Workflow is SUCCEEDED, but task "process" ended in FAILED
func TestMutation_WorkflowTerminationViolation(t *testing.T) {
	tmpDir := t.TempDir()
	createValidSyntheticRun(t, tmpDir)

	wfID := "wf-synthetic-1"
	taskFetch := &pb.Task{
		Spec:   &pb.TaskSpec{Id: "fetch", Type: "echo"},
		Status: &pb.TaskStatus{State: pb.TaskState_TASK_STATE_COMPLETED, CurrentAttempt: 1},
	}
	taskProcess := &pb.Task{
		Spec:   &pb.TaskSpec{Id: "process", Type: "echo", Dependencies: []string{"fetch"}},
		Status: &pb.TaskStatus{State: pb.TaskState_TASK_STATE_FAILED, CurrentAttempt: 1},
	}

	wfDump := []WorkflowDumpItem{
		{
			Workflow: &pb.Workflow{
				Spec: &pb.WorkflowSpec{Id: wfID, FailurePolicy: pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST, Tasks: []*pb.TaskSpec{taskFetch.Spec, taskProcess.Spec}},
				Status: &pb.WorkflowStatus{Id: wfID, State: pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED}, // Inconsistent state!
			},
			Tasks: []*pb.Task{taskFetch, taskProcess},
		},
	}
	wfData, _ := json.Marshal(wfDump)
	_ = os.WriteFile(filepath.Join(tmpDir, "workflows.json"), wfData, 0644)

	report, err := VerifyRunDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.AllPassed {
		t.Fatal("expected failure on inconsistent workflow termination, but checker passed!")
	}
	if report.WorkflowTermViolations == 0 {
		t.Fatalf("expected WorkflowTermViolations > 0, got %d", report.WorkflowTermViolations)
	}
}

// Mutation 5: Fencing Violation
// Event sequence 4 writes with old leader term 1 after event sequence 3 wrote with term 2 (zombie ex-leader write)
func TestMutation_FencingViolation(t *testing.T) {
	tmpDir := t.TempDir()
	createValidSyntheticRun(t, tmpDir)

	wfID := "wf-synthetic-1"
	corruptedEvents := []store.StateTransitionEvent{
		{SequenceNumber: 1, Timestamp: 1000, WorkflowID: wfID, FromState: "WORKFLOW_STATE_UNSPECIFIED", ToState: "WORKFLOW_STATE_RUNNING", LeaderTerm: 1},
		{SequenceNumber: 2, Timestamp: 1050, WorkflowID: wfID, TaskID: "fetch", AttemptID: 1, FromState: "TASK_STATE_READY", ToState: "TASK_STATE_ASSIGNED", LeaderTerm: 1},
		{SequenceNumber: 3, Timestamp: 1100, WorkflowID: wfID, TaskID: "fetch", AttemptID: 1, FromState: "TASK_STATE_ASSIGNED", ToState: "TASK_STATE_COMPLETED", LeaderTerm: 2}, // Term 2 established
		// Stale term 1 write from zombie ex-leader!
		{SequenceNumber: 4, Timestamp: 1150, WorkflowID: wfID, TaskID: "process", AttemptID: 1, FromState: "TASK_STATE_READY", ToState: "TASK_STATE_ASSIGNED", LeaderTerm: 1},
		{SequenceNumber: 5, Timestamp: 1200, WorkflowID: wfID, TaskID: "process", AttemptID: 1, FromState: "TASK_STATE_ASSIGNED", ToState: "TASK_STATE_COMPLETED", LeaderTerm: 2},
		{SequenceNumber: 6, Timestamp: 1250, WorkflowID: wfID, FromState: "WORKFLOW_STATE_RUNNING", ToState: "WORKFLOW_STATE_SUCCEEDED", LeaderTerm: 2},
	}
	writeEvents(t, tmpDir, corruptedEvents)

	report, err := VerifyRunDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.AllPassed {
		t.Fatal("expected failure on fencing violation, but checker passed!")
	}
	if report.FencingViolations == 0 {
		t.Fatalf("expected FencingViolations > 0, got %d", report.FencingViolations)
	}
}
