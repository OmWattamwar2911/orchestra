package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "orchestra/api/proto/orchestra/v1"
	"orchestra/internal/evidence"
	"orchestra/internal/store"
)

// FaultRecord represents an audit log entry for an injected failure.
type FaultRecord struct {
	TimestampNs int64     `json:"timestamp_ns"`
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Target      string    `json:"target"`
	Details     string    `json:"details"`
	Error       string    `json:"error,omitempty"`
}

// CheckerReport details the full invariant verification outcome for a single run.
type CheckerReport struct {
	RunDir         string `json:"run_dir"`
	Seed           int64  `json:"seed"`
	AllPassed      bool   `json:"all_passed"`
	TotalWorkflows int    `json:"total_workflows"`
	TotalTasks     int    `json:"total_tasks"`
	TotalEvents    int    `json:"total_events"`
	TotalEvidence  int    `json:"total_evidence"`

	// Invariant violation counters
	DependencyViolations   int `json:"dependency_violations"`
	SingleCommitViolations int `json:"single_commit_violations"`
	NoLostTasksViolations  int `json:"no_lost_tasks_violations"`
	WorkflowTermViolations int `json:"workflow_term_violations"`
	FencingViolations      int `json:"fencing_violations"`

	// Invariant violation descriptions
	Violations []string `json:"violations"`

	// Informational Duplicate Execution Tracking
	DuplicateExecutions  int            `json:"duplicate_executions"`
	DuplicateTaskDetails map[string]int `json:"duplicate_task_details,omitempty"`

	// Faults & Failover Telemetry
	FaultsByType    map[string]int `json:"faults_by_type"`
	LeaderFailovers int            `json:"leader_failovers"`
	MeanFailoverMs  float64        `json:"mean_failover_ms"`
	MaxFailoverMs   int64          `json:"max_failover_ms"`
}

type WorkflowDumpItem struct {
	Workflow *pb.Workflow `json:"workflow"`
	Tasks    []*pb.Task   `json:"tasks"`
}

// VerifyRunDir loads and verifies all artifacts in results/run-{seed}/ purely offline without network or live etcd.
func VerifyRunDir(runDir string) (*CheckerReport, error) {
	// Extract seed from directory name if applicable (e.g. "run-1001" -> 1001)
	var seed int64
	base := filepath.Base(runDir)
	if strings.HasPrefix(base, "run-") {
		s, _ := strconv.ParseInt(strings.TrimPrefix(base, "run-"), 10, 64)
		seed = s
	}

	report := &CheckerReport{
		RunDir:               runDir,
		Seed:                 seed,
		AllPassed:            true,
		Violations:           make([]string, 0),
		DuplicateTaskDetails: make(map[string]int),
		FaultsByType:         make(map[string]int),
	}

	// 1. Load etcd events
	eventsPath := filepath.Join(runDir, "events.jsonl")
	events, err := loadEvents(eventsPath)
	if err != nil {
		return nil, fmt.Errorf("failed loading %s: %w", eventsPath, err)
	}
	report.TotalEvents = len(events)

	// 2. Load worker evidence logs
	evidencePath := filepath.Join(runDir, "evidence.jsonl")
	evRecords, err := loadEvidence(evidencePath)
	if err != nil {
		return nil, fmt.Errorf("failed loading %s: %w", evidencePath, err)
	}
	report.TotalEvidence = len(evRecords)

	// 3. Load workflow definitions and final task states
	wfPath := filepath.Join(runDir, "workflows.json")
	wfDumps, err := loadWorkflows(wfPath)
	if err != nil {
		return nil, fmt.Errorf("failed loading %s: %w", wfPath, err)
	}
	report.TotalWorkflows = len(wfDumps)
	for _, w := range wfDumps {
		report.TotalTasks += len(w.Tasks)
	}

	// 4. Load faults log (if present)
	faultsPath := filepath.Join(runDir, "faults.jsonl")
	faults, _ := loadFaults(faultsPath)
	for _, f := range faults {
		report.FaultsByType[f.Type]++
	}

	// Verify all 6 invariants
	verifyFencing(events, report)
	verifyDependencyOrder(wfDumps, events, evRecords, report)
	verifySingleCommit(events, report)
	verifyNoLostTasks(wfDumps, report)
	verifyWorkflowTermination(wfDumps, report)
	verifyDuplicateExecutions(evRecords, events, report)
	computeFailoverMetrics(events, faults, report)

	if len(report.Violations) > 0 {
		report.AllPassed = false
	}

	return report, nil
}

// Invariant 1: DEPENDENCY ORDER
// No task has a committed start before all its dependencies have a committed completion.
func verifyDependencyOrder(wfDumps []WorkflowDumpItem, events []store.StateTransitionEvent, evRecords []evidence.EventRecord, report *CheckerReport) {
	type taskKey struct {
		wfID   string
		taskID string
	}

	// Record completion times from etcd event log
	completedTimes := make(map[taskKey]int64)
	firstCommittedStart := make(map[taskKey]int64)

	for _, ev := range events {
		k := taskKey{wfID: ev.WorkflowID, taskID: ev.TaskID}
		if ev.ToState == pb.TaskState_TASK_STATE_COMPLETED.String() {
			completedTimes[k] = ev.Timestamp
		}
		if ev.ToState == pb.TaskState_TASK_STATE_ASSIGNED.String() {
			if _, exists := firstCommittedStart[k]; !exists {
				firstCommittedStart[k] = ev.Timestamp
			}
		}
	}

	// Also index earliest worker evidence start timestamps (ms)
	evidenceStartTimes := make(map[taskKey]int64)
	for _, ev := range evRecords {
		if ev.Event == evidence.EventStart {
			k := taskKey{wfID: ev.WorkflowID, taskID: ev.TaskID}
			ms := ev.TimestampNs / 1e6
			if prev, exists := evidenceStartTimes[k]; !exists || ms < prev {
				evidenceStartTimes[k] = ms
			}
		}
	}

	for _, wd := range wfDumps {
		wf := wd.Workflow
		wfID := wf.GetSpec().GetId()

		for _, taskSpec := range wf.GetSpec().GetTasks() {
			taskID := taskSpec.GetId()
			k := taskKey{wfID: wfID, taskID: taskID}

			// Get earliest start timestamp (from committed ASSIGNED event or worker START evidence)
			startMs, hasStart := firstCommittedStart[k]
			if evMs, hasEv := evidenceStartTimes[k]; hasEv {
				if !hasStart || evMs < startMs {
					startMs = evMs
					hasStart = true
				}
			}

			if !hasStart {
				continue // Task was never started
			}

			for _, depID := range taskSpec.GetDependencies() {
				depKey := taskKey{wfID: wfID, taskID: depID}
				depCompleteMs, depFinished := completedTimes[depKey]

				if !depFinished {
					report.DependencyViolations++
					report.Violations = append(report.Violations,
						fmt.Sprintf("DEPENDENCY ORDER VIOLATION: task '%s' started in workflow '%s', but dependency '%s' never completed",
							taskID, wfID, depID),
					)
				} else if startMs < depCompleteMs {
					report.DependencyViolations++
					report.Violations = append(report.Violations,
						fmt.Sprintf("DEPENDENCY ORDER VIOLATION: task '%s' started at %d before dependency '%s' completed at %d (diff=%d ms)",
							taskID, startMs, depID, depCompleteMs, depCompleteMs-startMs),
					)
				}
			}
		}
	}
}

// Invariant 2: SINGLE COMMIT
// Delivery semantics: at-least-once execution with exactly-one committed result per task.
// Each task has at most one committed result, and exactly one if it reached COMPLETED.
func verifySingleCommit(events []store.StateTransitionEvent, report *CheckerReport) {
	type taskKey struct {
		wfID   string
		taskID string
	}

	terminalCommits := make(map[taskKey][]store.StateTransitionEvent)
	completedCommits := make(map[taskKey]int)

	for _, ev := range events {
		if ev.TaskID == "" {
			continue
		}
		k := taskKey{wfID: ev.WorkflowID, taskID: ev.TaskID}

		if isTerminalTaskStateStr(ev.ToState) {
			terminalCommits[k] = append(terminalCommits[k], ev)
			if ev.ToState == pb.TaskState_TASK_STATE_COMPLETED.String() {
				completedCommits[k]++
			}
		}
	}

	for k, list := range terminalCommits {
		if len(list) > 1 {
			report.SingleCommitViolations++
			report.Violations = append(report.Violations,
				fmt.Sprintf("SINGLE COMMIT VIOLATION: task '%s' in '%s' has %d committed terminal results (expected at most 1)",
					k.taskID, k.wfID, len(list)),
			)
		}
		if completedCount := completedCommits[k]; completedCount > 1 {
			report.SingleCommitViolations++
			report.Violations = append(report.Violations,
				fmt.Sprintf("SINGLE COMMIT VIOLATION: task '%s' in '%s' committed COMPLETED %d times (expected exactly 1)",
					k.taskID, k.wfID, completedCount),
			)
		}
	}
}

// Invariant 3: NO LOST TASKS
// Every task of every submitted workflow ended in a terminal state (COMPLETED, FAILED/DEAD_LETTER, SKIPPED, CANCELLED).
func verifyNoLostTasks(wfDumps []WorkflowDumpItem, report *CheckerReport) {
	for _, wd := range wfDumps {
		wfID := wd.Workflow.GetSpec().GetId()
		for _, task := range wd.Tasks {
			taskID := task.GetSpec().GetId()
			state := task.GetStatus().GetState()

			if !isTerminalTaskState(state) {
				report.NoLostTasksViolations++
				report.Violations = append(report.Violations,
					fmt.Sprintf("NO LOST TASKS VIOLATION: task '%s' in '%s' left in non-terminal state '%s'",
						taskID, wfID, state.String()),
				)
			}
		}
	}
}

// Invariant 4: WORKFLOW TERMINATION
// Every workflow reached a terminal state consistent with its task states and failure policy.
func verifyWorkflowTermination(wfDumps []WorkflowDumpItem, report *CheckerReport) {
	for _, wd := range wfDumps {
		wf := wd.Workflow
		wfID := wf.GetSpec().GetId()
		wfState := wf.GetStatus().GetState()
		policy := wf.GetSpec().GetFailurePolicy()

		if !isTerminalWorkflowState(wfState) {
			report.WorkflowTermViolations++
			report.Violations = append(report.Violations,
				fmt.Sprintf("WORKFLOW TERMINATION VIOLATION: workflow '%s' left in non-terminal state '%s'",
					wfID, wfState.String()),
			)
			continue
		}

		// Count task states in this workflow
		allCompleted := true
		hasFailure := false

		for _, t := range wd.Tasks {
			st := t.GetStatus().GetState()
			if st != pb.TaskState_TASK_STATE_COMPLETED {
				allCompleted = false
			}
			if st == pb.TaskState_TASK_STATE_FAILED || st == pb.TaskState_TASK_STATE_DEAD_LETTER {
				hasFailure = true
			}
		}

		if wfState == pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED {
			if !allCompleted {
				report.WorkflowTermViolations++
				report.Violations = append(report.Violations,
					fmt.Sprintf("WORKFLOW TERMINATION VIOLATION: workflow '%s' marked SUCCEEDED but not all tasks reached COMPLETED", wfID),
				)
			}
		} else if wfState == pb.WorkflowState_WORKFLOW_STATE_FAILED {
			if !hasFailure {
				report.WorkflowTermViolations++
				report.Violations = append(report.Violations,
					fmt.Sprintf("WORKFLOW TERMINATION VIOLATION: workflow '%s' marked FAILED but no tasks experienced FAILURE/DEAD_LETTER", wfID),
				)
			}
			if policy == pb.FailurePolicy_FAILURE_POLICY_FAIL_FAST {
				// Assert uncompleted tasks were cancelled or terminated
				for _, t := range wd.Tasks {
					st := t.GetStatus().GetState()
					if st == pb.TaskState_TASK_STATE_PENDING || st == pb.TaskState_TASK_STATE_READY || st == pb.TaskState_TASK_STATE_ASSIGNED || st == pb.TaskState_TASK_STATE_RUNNING {
						report.WorkflowTermViolations++
						report.Violations = append(report.Violations,
							fmt.Sprintf("WORKFLOW TERMINATION VIOLATION: FAIL_FAST policy violated; task '%s' not cancelled in '%s'", t.GetSpec().GetId(), wfID),
						)
					}
				}
			}
		}
	}
}

// Invariant 5: FENCING
// No state write from a leader term older than the term that was current at that moment.
// Verify sequence numbers and monotonicity of leader terms.
func verifyFencing(events []store.StateTransitionEvent, report *CheckerReport) {
	currentMaxTerm := int64(0)

	for _, ev := range events {
		if ev.LeaderTerm < currentMaxTerm {
			report.FencingViolations++
			report.Violations = append(report.Violations,
				fmt.Sprintf("FENCING VIOLATION (zombie write): event seq %d (wf=%s, task=%s) wrote with term %d after term %d was already established",
					ev.SequenceNumber, ev.WorkflowID, ev.TaskID, ev.LeaderTerm, currentMaxTerm),
			)
		} else if ev.LeaderTerm > currentMaxTerm {
			currentMaxTerm = ev.LeaderTerm
		}
	}
}

// Invariant 6: DUPLICATE EXECUTION REPORT (informational, not a failure)
// Count tasks executed >1 times due to retries or worker kills, and confirm single commit for each.
func verifyDuplicateExecutions(evRecords []evidence.EventRecord, events []store.StateTransitionEvent, report *CheckerReport) {
	type taskKey struct {
		wfID   string
		taskID string
	}

	startCounts := make(map[taskKey]int)
	for _, ev := range evRecords {
		if ev.Event == evidence.EventStart {
			k := taskKey{wfID: ev.WorkflowID, taskID: ev.TaskID}
			startCounts[k]++
		}
	}

	for k, count := range startCounts {
		if count > 1 {
			report.DuplicateExecutions++
			report.DuplicateTaskDetails[fmt.Sprintf("%s/%s", k.wfID, k.taskID)] = count
		}
	}
}

func computeFailoverMetrics(events []store.StateTransitionEvent, faults []FaultRecord, report *CheckerReport) {
	// Identify term changes
	termFirstEventTime := make(map[int64]int64)
	for _, ev := range events {
		if _, exists := termFirstEventTime[ev.LeaderTerm]; !exists {
			termFirstEventTime[ev.LeaderTerm] = ev.Timestamp
		}
	}

	terms := make([]int64, 0, len(termFirstEventTime))
	for t := range termFirstEventTime {
		terms = append(terms, t)
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i] < terms[j] })

	if len(terms) > 1 {
		report.LeaderFailovers = len(terms) - 1
	}

	// Calculate failover delays based on leader kill/pause faults
	var totalFailoverMs int64
	var countFailovers int

	for i := 1; i < len(terms); i++ {
		tNew := terms[i]
		tFirstMs := termFirstEventTime[tNew]

		// Find fault preceding this term transition
		for _, f := range faults {
			if strings.Contains(f.Type, "LEADER") {
				faultMs := f.TimestampNs / 1e6
				if faultMs <= tFirstMs && (tFirstMs-faultMs) < 30000 {
					delay := tFirstMs - faultMs
					if delay > report.MaxFailoverMs {
						report.MaxFailoverMs = delay
					}
					totalFailoverMs += delay
					countFailovers++
					break
				}
			}
		}
	}

	if countFailovers > 0 {
		report.MeanFailoverMs = float64(totalFailoverMs) / float64(countFailovers)
	}
}

// Helpers
func loadEvents(path string) ([]store.StateTransitionEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []store.StateTransitionEvent
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev store.StateTransitionEvent
		if err := json.Unmarshal([]byte(line), &ev); err == nil {
			events = append(events, ev)
		}
	}
	return events, scanner.Err()
}

func loadEvidence(path string) ([]evidence.EventRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []evidence.EventRecord
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var r evidence.EventRecord
		if err := json.Unmarshal([]byte(line), &r); err == nil {
			records = append(records, r)
		}
	}
	return records, scanner.Err()
}

func loadWorkflows(path string) ([]WorkflowDumpItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var items []WorkflowDumpItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func loadFaults(path string) ([]FaultRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var faults []FaultRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var fr FaultRecord
		if err := json.Unmarshal([]byte(line), &fr); err == nil {
			faults = append(faults, fr)
		}
	}
	return faults, nil
}

func isTerminalTaskState(st pb.TaskState) bool {
	switch st {
	case pb.TaskState_TASK_STATE_COMPLETED,
		pb.TaskState_TASK_STATE_FAILED,
		pb.TaskState_TASK_STATE_DEAD_LETTER,
		pb.TaskState_TASK_STATE_SKIPPED,
		pb.TaskState_TASK_STATE_CANCELLED:
		return true
	default:
		return false
	}
}

func isTerminalTaskStateStr(st string) bool {
	switch st {
	case "TASK_STATE_COMPLETED",
		"TASK_STATE_FAILED",
		"TASK_STATE_DEAD_LETTER",
		"TASK_STATE_SKIPPED",
		"TASK_STATE_CANCELLED":
		return true
	default:
		return false
	}
}

func isTerminalWorkflowState(st pb.WorkflowState) bool {
	switch st {
	case pb.WorkflowState_WORKFLOW_STATE_SUCCEEDED,
		pb.WorkflowState_WORKFLOW_STATE_FAILED,
		pb.WorkflowState_WORKFLOW_STATE_CANCELLED:
		return true
	default:
		return false
	}
}
