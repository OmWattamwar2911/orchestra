package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type AggregateReport struct {
	TotalRuns              int              `json:"total_runs"`
	PassedRuns             int              `json:"passed_runs"`
	FailedRuns             int              `json:"failed_runs"`
	TotalWorkflows         int              `json:"total_workflows"`
	TotalTasks             int              `json:"total_tasks"`
	TotalEvents            int              `json:"total_events"`
	TotalEvidenceRecords   int              `json:"total_evidence_records"`
	TotalFaults            map[string]int   `json:"total_faults"`
	TotalLeaderFailovers   int              `json:"total_leader_failovers"`
	MeanFailoverMs         float64          `json:"mean_failover_ms"`
	MaxFailoverMs          int64            `json:"max_failover_ms"`
	TotalDuplicateExecs    int              `json:"total_duplicate_executions"`
	DependencyViolations   int              `json:"dependency_violations"`
	SingleCommitViolations int              `json:"single_commit_violations"`
	NoLostTasksViolations  int              `json:"no_lost_tasks_violations"`
	WorkflowTermViolations int              `json:"workflow_term_violations"`
	FencingViolations      int              `json:"fencing_violations"`
	RunReports             []*CheckerReport `json:"run_reports"`
}

func main() {
	var (
		runDirFlag     = flag.String("run-dir", "", "Path to a specific run directory (e.g. results/run-1001)")
		resultsDirFlag = flag.String("results-dir", "results", "Path to results directory containing multiple run-* directories")
		outputPath     = flag.String("output", "", "Optional path to save JSON verification report")
	)
	flag.Parse()

	var runDirs []string

	if *runDirFlag != "" {
		runDirs = []string{*runDirFlag}
	} else {
		// Scan results-dir for run-* subdirectories
		entries, err := os.ReadDir(*resultsDirFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot read results directory %s: %v\n", *resultsDirFlag, err)
			os.Exit(1)
		}
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "run-") {
				runDirs = append(runDirs, filepath.Join(*resultsDirFlag, e.Name()))
			}
		}
		sort.Strings(runDirs)
	}

	if len(runDirs) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no run directories found to verify in %s\n", *resultsDirFlag)
		os.Exit(1)
	}

	agg := &AggregateReport{
		TotalFaults: make(map[string]int),
		RunReports:  make([]*CheckerReport, 0, len(runDirs)),
	}

	var totalFailoverMsSum float64
	var totalFailoverCount int

	fmt.Println("=======================================================================================================================")
	fmt.Printf(" ORCHESTRA POST-CHAOS OFFLINE ARTIFACT INVARIANT CHECKER\n")
	fmt.Printf(" Inspecting %d run artifact directories (NO live cluster required)\n", len(runDirs))
	fmt.Println("=======================================================================================================================")

	for _, dir := range runDirs {
		report, err := VerifyRunDir(dir)
		if err != nil {
			fmt.Printf("Error verifying %s: %v\n", dir, err)
			agg.FailedRuns++
			continue
		}

		agg.RunReports = append(agg.RunReports, report)
		agg.TotalRuns++
		if report.AllPassed {
			agg.PassedRuns++
		} else {
			agg.FailedRuns++
		}

		agg.TotalWorkflows += report.TotalWorkflows
		agg.TotalTasks += report.TotalTasks
		agg.TotalEvents += report.TotalEvents
		agg.TotalEvidenceRecords += report.TotalEvidence
		agg.TotalDuplicateExecs += report.DuplicateExecutions
		agg.DependencyViolations += report.DependencyViolations
		agg.SingleCommitViolations += report.SingleCommitViolations
		agg.NoLostTasksViolations += report.NoLostTasksViolations
		agg.WorkflowTermViolations += report.WorkflowTermViolations
		agg.FencingViolations += report.FencingViolations

		for fType, count := range report.FaultsByType {
			agg.TotalFaults[fType] += count
		}

		agg.TotalLeaderFailovers += report.LeaderFailovers
		if report.MaxFailoverMs > agg.MaxFailoverMs {
			agg.MaxFailoverMs = report.MaxFailoverMs
		}
		if report.LeaderFailovers > 0 && report.MeanFailoverMs > 0 {
			totalFailoverMsSum += report.MeanFailoverMs * float64(report.LeaderFailovers)
			totalFailoverCount += report.LeaderFailovers
		}

		// Save per-run report
		reportData, _ := json.MarshalIndent(report, "", "  ")
		_ = os.WriteFile(filepath.Join(dir, "checker_report.json"), reportData, 0644)

		statusStr := "PASS"
		if !report.AllPassed {
			statusStr = "FAIL"
		}
		fmt.Printf("[%s] Run %s | WFs: %3d | Tasks: %4d | Evts: %4d | Evid: %4d | Retried: %2d | Faults: %2d | Failovers: %d\n",
			statusStr, filepath.Base(dir), report.TotalWorkflows, report.TotalTasks, report.TotalEvents,
			report.TotalEvidence, report.DuplicateExecutions, sumMap(report.FaultsByType), report.LeaderFailovers)

		if len(report.Violations) > 0 {
			for _, v := range report.Violations {
				fmt.Printf("    -> %s\n", v)
			}
		}
	}

	if totalFailoverCount > 0 {
		agg.MeanFailoverMs = totalFailoverMsSum / float64(totalFailoverCount)
	}

	// Print aggregate summary table
	fmt.Println("\n=======================================================================================================================")
	fmt.Printf(" AGGREGATE CHAOS INVARIANT VERIFICATION SUMMARY\n")
	fmt.Println("=======================================================================================================================")
	fmt.Printf(" Total Runs Verified:         %d (%d passed, %d failed)\n", agg.TotalRuns, agg.PassedRuns, agg.FailedRuns)
	fmt.Printf(" Total Workflows Submitted:   %d\n", agg.TotalWorkflows)
	fmt.Printf(" Total Tasks Executed:        %d\n", agg.TotalTasks)
	fmt.Printf(" Total Events in Event Log:   %d\n", agg.TotalEvents)
	fmt.Printf(" Total Evidence Log Records:  %d\n", agg.TotalEvidenceRecords)
	fmt.Printf(" Duplicate Task Executions:   %d (retried due to worker/leader kill; exactly-one committed)\n", agg.TotalDuplicateExecs)
	fmt.Printf(" Leader Failovers Detected:   %d (Mean: %.1f ms, Max: %d ms)\n", agg.TotalLeaderFailovers, agg.MeanFailoverMs, agg.MaxFailoverMs)
	fmt.Println("-----------------------------------------------------------------------------------------------------------------------")
	fmt.Println(" Faults Injected by Type:")
	for fType, count := range agg.TotalFaults {
		fmt.Printf("   - %-30s : %d\n", fType, count)
	}
	fmt.Println("-----------------------------------------------------------------------------------------------------------------------")
	fmt.Println(" Invariant Verification Outcomes:")
	fmt.Printf("   1. DEPENDENCY ORDER        : %d violations %s\n", agg.DependencyViolations, statusTag(agg.DependencyViolations))
	fmt.Printf("   2. SINGLE COMMIT           : %d violations %s\n", agg.SingleCommitViolations, statusTag(agg.SingleCommitViolations))
	fmt.Printf("   3. NO LOST TASKS           : %d violations %s\n", agg.NoLostTasksViolations, statusTag(agg.NoLostTasksViolations))
	fmt.Printf("   4. WORKFLOW TERMINATION    : %d violations %s\n", agg.WorkflowTermViolations, statusTag(agg.WorkflowTermViolations))
	fmt.Printf("   5. FENCING (no zombie write): %d violations %s\n", agg.FencingViolations, statusTag(agg.FencingViolations))
	fmt.Println("=======================================================================================================================")

	if *outputPath != "" {
		data, _ := json.MarshalIndent(agg, "", "  ")
		_ = os.WriteFile(*outputPath, data, 0644)
		fmt.Printf("Full aggregate verification JSON saved to: %s\n", *outputPath)
	}

	if agg.FailedRuns > 0 {
		os.Exit(2)
	}
}

func sumMap(m map[string]int) int {
	s := 0
	for _, v := range m {
		s += v
	}
	return s
}

func statusTag(violations int) string {
	if violations == 0 {
		return "[PASSED]"
	}
	return "[FAILED]"
}
