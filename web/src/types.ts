export type TaskStateNumber = 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9;

export const TaskState = {
  UNSPECIFIED: 0,
  PENDING: 1,
  READY: 2,
  ASSIGNED: 3,
  RUNNING: 4,
  COMPLETED: 5,
  FAILED: 6,
  SKIPPED: 7,
  CANCELLED: 8,
  DEAD_LETTER: 9,
} as const;

export const TaskStateNames: Record<number, string> = {
  0: 'UNSPECIFIED',
  1: 'PENDING',
  2: 'READY',
  3: 'ASSIGNED',
  4: 'RUNNING',
  5: 'COMPLETED',
  6: 'FAILED',
  7: 'SKIPPED',
  8: 'CANCELLED',
  9: 'DEAD_LETTER',
};

export const WorkflowState = {
  UNSPECIFIED: 0,
  RUNNING: 1,
  SUCCEEDED: 2,
  FAILED: 3,
  CANCELLED: 4,
} as const;

export const WorkflowStateNames: Record<number, string> = {
  0: 'UNSPECIFIED',
  1: 'RUNNING',
  2: 'SUCCEEDED',
  3: 'FAILED',
  4: 'CANCELLED',
};

export const FailurePolicy = {
  FAIL_FAST: 1,
  CONTINUE: 2,
} as const;

export interface TaskSpec {
  id: string;
  type?: string;
  payload?: string;
  timeout_seconds?: number;
  max_retries?: number;
  dependencies?: string[];
}

export interface TaskStatus {
  id: string;
  workflow_id: string;
  state: number;
  current_attempt: number;
  assigned_worker: string;
  started_at: number;
  completed_at: number;
  error_message: string;
  mod_revision?: number;
}

export interface Task {
  spec: TaskSpec;
  status: TaskStatus;
}

export interface WorkflowSpec {
  id: string;
  name: string;
  failure_policy: number;
  tasks: TaskSpec[];
}

export interface WorkflowStatus {
  id: string;
  state: number;
  created_at: number;
  completed_at: number;
  total_tasks: number;
  completed_tasks: number;
  failed_tasks: number;
}

export interface Workflow {
  spec: WorkflowSpec;
  status: WorkflowStatus;
}

export interface WorkflowWithTasks {
  workflow: Workflow;
  tasks: Task[];
}

export interface WorkerInfo {
  worker_id: string;
  capacity?: number;
  current_tasks?: number;
  running_tasks?: number;
  supported_types?: string[];
  last_heartbeat?: number;
  active?: boolean;
  address?: string;
}

export interface OrchestratorNode {
  node_id: string;
  addr: string;
  http_addr: string;
  is_leader: boolean;
  role: 'LEADER' | 'STANDBY';
  status: string;
}

export interface EventRecord {
  sequence?: number;
  timestamp?: number;
  type?: string;
  workflow_id?: string;
  task_id?: string;
  attempt_id?: number;
  worker_id?: string;
  old_state?: number | string;
  new_state?: number | string;
  term?: number;
  error?: string;
  [key: string]: any;
}

export interface ClusterStatus {
  node_id: string;
  is_leader: boolean;
  fencing_token: number;
  leader_key: string;
  leader?: {
    node_id: string;
    grpc_addr: string;
    http_addr: string;
    started_at: number;
  };
  term: number;
  nodes: OrchestratorNode[];
  worker_count: number;
  workers: WorkerInfo[];
  queue_depth: number;
  total_workflows: number;
  recent_events: EventRecord[];
  timestamp: number;
}
