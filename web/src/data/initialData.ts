import { ClusterStatus, WorkflowWithTasks } from '../types';

export const fallbackCluster: ClusterStatus = {
  node_id: 'orchestrator-1',
  is_leader: true,
  fencing_token: 1,
  leader_key: '/orchestra/leader/election/orchestrator-1',
  leader: {
    node_id: 'orchestrator-1',
    grpc_addr: 'orchestrator-1:50051',
    http_addr: 'http://orchestrator-1:8080',
    started_at: Date.now() - 60000,
  },
  term: 1,
  nodes: [
    { node_id: 'orchestrator-1', addr: 'orchestrator-1:50051', http_addr: '', is_leader: true, role: 'LEADER', status: 'UP' },
    { node_id: 'orchestrator-2', addr: 'orchestrator-2:50051', http_addr: '', is_leader: false, role: 'STANDBY', status: 'UP' },
    { node_id: 'orchestrator-3', addr: 'orchestrator-3:50051', http_addr: '', is_leader: false, role: 'STANDBY', status: 'UP' },
  ],
  worker_count: 5,
  workers: [
    { worker_id: 'worker-1', capacity: 4, current_tasks: 1, running_tasks: 1, active: true },
    { worker_id: 'worker-2', capacity: 4, current_tasks: 1, running_tasks: 1, active: true },
    { worker_id: 'worker-3', capacity: 4, current_tasks: 0, running_tasks: 0, active: true },
    { worker_id: 'worker-4', capacity: 4, current_tasks: 0, running_tasks: 0, active: true },
    { worker_id: 'worker-5', capacity: 4, current_tasks: 0, running_tasks: 0, active: true },
  ],
  queue_depth: 0,
  total_workflows: 1,
  recent_events: [
    { timestamp: Date.now() - 12000, type: 'TASK_COMPLETED', workflow_id: 'diamond-pipeline-sample', task_id: 'fetch', old_state: 'RUNNING', new_state: 'COMPLETED', worker_id: 'worker-1', term: 1 },
    { timestamp: Date.now() - 6000, type: 'TASK_SCHEDULED', workflow_id: 'diamond-pipeline-sample', task_id: 'transformA', old_state: 'READY', new_state: 'RUNNING', worker_id: 'worker-1', term: 1 },
    { timestamp: Date.now() - 4000, type: 'TASK_SCHEDULED', workflow_id: 'diamond-pipeline-sample', task_id: 'transformB', old_state: 'READY', new_state: 'RUNNING', worker_id: 'worker-2', term: 1 },
  ],
  timestamp: Date.now(),
};

export const fallbackWorkflows: WorkflowWithTasks[] = [
  {
    workflow: {
      spec: {
        id: 'diamond-pipeline-sample',
        name: 'Diamond Invariant Pipeline',
        failure_policy: 1,
        tasks: [],
      },
      status: {
        id: 'diamond-pipeline-sample',
        state: 1,
        created_at: Date.now() - 20000,
        completed_at: 0,
        total_tasks: 5,
        completed_tasks: 1,
        failed_tasks: 0,
      },
    },
    tasks: [
      {
        spec: { id: 'fetch', type: 'mock', payload: 'fetch raw dataset', timeout_seconds: 20, max_retries: 2, dependencies: [] },
        status: { id: 'fetch', workflow_id: 'diamond-pipeline-sample', state: 5, current_attempt: 1, assigned_worker: 'worker-1', started_at: Date.now() - 20000, completed_at: Date.now() - 15000, error_message: '' },
      },
      {
        spec: { id: 'transformA', type: 'sleep', payload: 'sleep: 3s', timeout_seconds: 20, max_retries: 2, dependencies: ['fetch'] },
        status: { id: 'transformA', workflow_id: 'diamond-pipeline-sample', state: 4, current_attempt: 1, assigned_worker: 'worker-1', started_at: Date.now() - 10000, completed_at: 0, error_message: '' },
      },
      {
        spec: { id: 'transformB', type: 'sleep', payload: 'sleep: 4s', timeout_seconds: 20, max_retries: 2, dependencies: ['fetch'] },
        status: { id: 'transformB', workflow_id: 'diamond-pipeline-sample', state: 4, current_attempt: 1, assigned_worker: 'worker-2', started_at: Date.now() - 8000, completed_at: 0, error_message: '' },
      },
      {
        spec: { id: 'merge', type: 'mock', payload: 'merge transformed chunks', timeout_seconds: 20, max_retries: 2, dependencies: ['transformA', 'transformB'] },
        status: { id: 'merge', workflow_id: 'diamond-pipeline-sample', state: 1, current_attempt: 0, assigned_worker: '', started_at: 0, completed_at: 0, error_message: '' },
      },
      {
        spec: { id: 'publish', type: 'mock', payload: 'publish aggregated result', timeout_seconds: 20, max_retries: 2, dependencies: ['merge'] },
        status: { id: 'publish', workflow_id: 'diamond-pipeline-sample', state: 1, current_attempt: 0, assigned_worker: '', started_at: 0, completed_at: 0, error_message: '' },
      },
    ],
  },
];
