package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// --- Orchestrator Metrics ---

	// WorkflowsSubmittedTotal counts total number of workflows submitted to the orchestrator.
	WorkflowsSubmittedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "workflows_submitted_total",
		Help: "Total number of workflows submitted to the orchestrator",
	})

	// WorkflowsSucceededTotal counts workflows that resolved to terminal state SUCCEEDED.
	WorkflowsSucceededTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "workflows_succeeded_total",
		Help: "Total number of workflows successfully completed",
	})

	// WorkflowsFailedTotal counts workflows that resolved to terminal state FAILED or CANCELLED.
	WorkflowsFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "workflows_failed_total",
		Help: "Total number of workflows failed or cancelled",
	})

	// TasksCompletedTotal counts task executions that completed successfully.
	TasksCompletedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tasks_completed_total",
		Help: "Total number of task attempts that completed successfully",
	})

	// TasksFailedTotal counts task execution attempts that failed or timed out.
	TasksFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tasks_failed_total",
		Help: "Total number of task attempts that resulted in failure or timeout",
	})

	// TasksDeadLetterTotal counts tasks moved to dead letter queue after exhausting max retries.
	TasksDeadLetterTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tasks_dead_letter_total",
		Help: "Total number of tasks transitioned to dead letter after exhausting retries",
	})

	// StaleResultsRejectedTotal counts late, duplicate, or mismatched attempt reports rejected by the store.
	StaleResultsRejectedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stale_results_rejected_total",
		Help: "Total number of task results rejected due to stale attempt ID or terminal state",
	})

	// FencedWriteRejectionsTotal counts leader writes rejected by etcd election fencing checks.
	FencedWriteRejectionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fenced_write_rejections_total",
		Help: "Total number of leader etcd transactions rejected by fencing checks",
	})

	// QueueDepth tracks the current number of ready tasks waiting in the scheduler priority queue.
	QueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "queue_depth",
		Help: "Current depth of ready tasks in the scheduler priority queue",
	})

	// IsLeader indicates whether this orchestrator node holds the active cluster leader lock.
	IsLeader = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "is_leader",
		Help: "Leadership indicator (1 if this node is the active leader, 0 otherwise)",
	})

	// LeaderTerm reports the etcd election key create revision for the active leader.
	LeaderTerm = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "leader_term",
		Help: "Active leader fencing revision/term in etcd",
	})

	// LeaderElectionsTotal counts the number of times this node assumed cluster leadership.
	LeaderElectionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "leader_elections_total",
		Help: "Total number of times this node won election as cluster leader",
	})

	// WorkerCount tracks the number of live workers currently connected and registered with valid leases.
	WorkerCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "worker_count",
		Help: "Number of active live workers currently registered",
	})

	// TaskLatencySeconds records execution duration of tasks from start to result commitment.
	TaskLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "task_latency_seconds",
		Help:    "Task execution latency in seconds from start to result commit",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0},
	})

	// WorkflowDurationSeconds records total duration of workflows from submission to terminal state.
	WorkflowDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "workflow_duration_seconds",
		Help:    "End-to-end workflow execution duration in seconds",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0},
	})

	// --- Worker Metrics ---

	// TasksRunning tracks the current number of tasks executing concurrently on this worker.
	TasksRunning = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tasks_running",
		Help: "Current number of tasks actively executing on this worker",
	})

	// TasksExecutedTotal tracks total tasks executed on this worker by outcome.
	TasksExecutedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tasks_executed_total",
		Help: "Total number of tasks executed by this worker partitioned by outcome",
	}, []string{"outcome"})
)
