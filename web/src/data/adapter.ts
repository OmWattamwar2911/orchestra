import { ClusterStatus, WorkflowWithTasks, EventRecord } from '../types';

export interface ChaosResult {
  status: string;
  leader_id?: string;
  new_leader?: string;
  worker_id?: string;
  node_id?: string;
  term?: number;
  killed_by_docker?: boolean;
  timestamp: number;
  error?: string;
}

export interface ClusterAdapter {
  getClusterStatus(): Promise<ClusterStatus>;
  getWorkflows(): Promise<WorkflowWithTasks[]>;
  getWorkflow(id: string): Promise<WorkflowWithTasks | null>;
  submitWorkflow(spec: string): Promise<{ workflow_id: string }>;
  cancelWorkflow(id: string): Promise<void>;
  killLeader(): Promise<ChaosResult>;
  killWorker(workerId?: string): Promise<ChaosResult>;
  pauseLeader(): Promise<ChaosResult>;
  restartEtcdNode?(nodeId?: string): Promise<ChaosResult>;
  subscribeEvents(onEvent: (event: EventRecord) => void): () => void;
  isSimulator(): boolean;
}
