import { ClusterAdapter, ChaosResult } from './adapter';
import { ClusterStatus, WorkflowWithTasks, EventRecord, TaskState } from '../types';

// Seeded Mulberry32 PRNG for deterministic, reproducible simulations
export function createMulberry32(seed: number) {
  let s = seed >>> 0;
  return function next(): number {
    s = (s + 0x6D2B79F5) >>> 0;
    let t = Math.imul(s ^ (s >>> 15), 1 | s);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export class SimulatorAdapter implements ClusterAdapter {
  private rng = createMulberry32(4001);
  private term = 1;
  private currentLeader = 'orchestrator-1';
  private subscribers = new Set<(event: EventRecord) => void>();
  private startTime = Date.now();
  private timer: any = null;

  private orchestrators = [
    { node_id: 'orchestrator-1', addr: 'orchestrator-1:50051', is_leader: true, role: 'LEADER', status: 'UP' },
    { node_id: 'orchestrator-2', addr: 'orchestrator-2:50051', is_leader: false, role: 'STANDBY', status: 'UP' },
    { node_id: 'orchestrator-3', addr: 'orchestrator-3:50051', is_leader: false, role: 'STANDBY', status: 'UP' },
  ];

  private workers = [
    { worker_id: 'worker-1', capacity: 4, current_tasks: 1, running_tasks: 1, active: true, lease_expires_at: Date.now() + 5000 },
    { worker_id: 'worker-2', capacity: 4, current_tasks: 1, running_tasks: 1, active: true, lease_expires_at: Date.now() + 5000 },
    { worker_id: 'worker-3', capacity: 4, current_tasks: 0, running_tasks: 0, active: true, lease_expires_at: Date.now() + 5000 },
    { worker_id: 'worker-4', capacity: 4, current_tasks: 0, running_tasks: 0, active: true, lease_expires_at: Date.now() + 5000 },
    { worker_id: 'worker-5', capacity: 4, current_tasks: 0, running_tasks: 0, active: true, lease_expires_at: Date.now() + 5000 },
  ];

  private workflows: WorkflowWithTasks[] = [
    {
      workflow: {
        spec: { id: 'diamond-pipeline-sample', name: 'Diamond Invariant Pipeline', failure_policy: 1, tasks: [] },
        status: { id: 'diamond-pipeline-sample', state: 1, created_at: Date.now() - 25000, completed_at: 0, total_tasks: 5, completed_tasks: 1, failed_tasks: 0 },
      },
      tasks: [
        {
          spec: { id: 'fetch', type: 'sleep', payload: 'sleep: 2s', timeout_seconds: 20, max_retries: 2, dependencies: [] },
          status: { id: 'fetch', workflow_id: 'diamond-pipeline-sample', state: TaskState.COMPLETED, current_attempt: 1, assigned_worker: 'worker-1', started_at: Date.now() - 24000, completed_at: Date.now() - 20000, error_message: '' },
        },
        {
          spec: { id: 'transformA', type: 'sleep', payload: 'sleep: 4s', timeout_seconds: 20, max_retries: 2, dependencies: ['fetch'] },
          status: { id: 'transformA', workflow_id: 'diamond-pipeline-sample', state: TaskState.RUNNING, current_attempt: 1, assigned_worker: 'worker-1', started_at: Date.now() - 12000, completed_at: 0, error_message: '' },
        },
        {
          spec: { id: 'transformB', type: 'sleep', payload: 'sleep: 5s', timeout_seconds: 20, max_retries: 2, dependencies: ['fetch'] },
          status: { id: 'transformB', workflow_id: 'diamond-pipeline-sample', state: TaskState.RUNNING, current_attempt: 1, assigned_worker: 'worker-2', started_at: Date.now() - 10000, completed_at: 0, error_message: '' },
        },
        {
          spec: { id: 'merge', type: 'sleep', payload: 'sleep: 3s', timeout_seconds: 20, max_retries: 2, dependencies: ['transformA', 'transformB'] },
          status: { id: 'merge', workflow_id: 'diamond-pipeline-sample', state: TaskState.READY, current_attempt: 0, assigned_worker: '', started_at: 0, completed_at: 0, error_message: '' },
        },
        {
          spec: { id: 'publish', type: 'sleep', payload: 'sleep: 2s', timeout_seconds: 20, max_retries: 2, dependencies: ['merge'] },
          status: { id: 'publish', workflow_id: 'diamond-pipeline-sample', state: TaskState.PENDING, current_attempt: 0, assigned_worker: '', started_at: 0, completed_at: 0, error_message: '' },
        },
      ],
    },
  ];

  private recentEvents: EventRecord[] = [
    { timestamp: Date.now() - 24000, type: 'TASK_SCHEDULED', workflow_id: 'diamond-pipeline-sample', task_id: 'fetch', new_state: 'RUNNING', worker_id: 'worker-1', term: 1 },
    { timestamp: Date.now() - 20000, type: 'TASK_COMPLETED', workflow_id: 'diamond-pipeline-sample', task_id: 'fetch', old_state: 'RUNNING', new_state: 'COMPLETED', worker_id: 'worker-1', term: 1 },
    { timestamp: Date.now() - 12000, type: 'TASK_SCHEDULED', workflow_id: 'diamond-pipeline-sample', task_id: 'transformA', new_state: 'RUNNING', worker_id: 'worker-1', term: 1 },
    { timestamp: Date.now() - 10000, type: 'TASK_SCHEDULED', workflow_id: 'diamond-pipeline-sample', task_id: 'transformB', new_state: 'RUNNING', worker_id: 'worker-2', term: 1 },
  ];

  constructor() {
    this.startSimulationLoop();
  }

  isSimulator(): boolean {
    return true;
  }

  private emitEvent(event: EventRecord) {
    this.recentEvents = [event, ...this.recentEvents.slice(0, 149)];
    this.subscribers.forEach(cb => {
      try {
        cb(event);
      } catch {}
    });
  }

  private startSimulationLoop() {
    if (this.timer) return;

    this.timer = setInterval(() => {
      // 1. Refresh worker heartbeat leases
      const now = Date.now();
      this.workers.forEach(w => {
        if (w.active) {
          w.lease_expires_at = now + 5000;
        }
      });

      // 2. Advance tasks realistically
      this.workflows.forEach(wf => {
        if (wf.workflow.status.state === 2 || wf.workflow.status.state === 3) return; // terminal

        const tasks = wf.tasks;
        const completedIds = new Set(
          tasks.filter(t => t.status.state === TaskState.COMPLETED).map(t => t.spec.id)
        );

        tasks.forEach(t => {
          // If task is RUNNING, advance to COMPLETED after duration
          if (t.status.state === TaskState.RUNNING) {
            const runningDuration = now - (t.status.started_at || now);
            if (runningDuration >= 6000) {
              t.status.state = TaskState.COMPLETED;
              t.status.completed_at = now;
              this.emitEvent({
                timestamp: now,
                type: 'TASK_COMPLETED',
                workflow_id: wf.workflow.spec.id,
                task_id: t.spec.id,
                old_state: 'RUNNING',
                new_state: 'COMPLETED',
                worker_id: t.status.assigned_worker,
                term: this.term,
              });
            }
          }

          // If task is READY or PENDING, check if dependencies completed
          if (t.status.state === TaskState.READY || t.status.state === TaskState.PENDING) {
            const deps = t.spec.dependencies || [];
            const depsMet = deps.every(d => completedIds.has(d));
            if (depsMet && t.status.state === TaskState.PENDING) {
              t.status.state = TaskState.READY;
            }

            if (t.status.state === TaskState.READY) {
              // Assign to available active worker with lowest utilization
              const availableWorker = this.workers.find(w => w.active && (w.current_tasks || 0) < w.capacity);
              if (availableWorker) {
                t.status.state = TaskState.RUNNING;
                t.status.assigned_worker = availableWorker.worker_id;
                t.status.started_at = now;
                t.status.current_attempt = Math.max(1, t.status.current_attempt || 1);
                this.emitEvent({
                  timestamp: now,
                  type: 'TASK_SCHEDULED',
                  workflow_id: wf.workflow.spec.id,
                  task_id: t.spec.id,
                  new_state: 'RUNNING',
                  worker_id: availableWorker.worker_id,
                  term: this.term,
                });
              }
            }
          }
        });

        // Update workflow completion stats
        const doneCount = tasks.filter(t => t.status.state === TaskState.COMPLETED).length;
        wf.workflow.status.completed_tasks = doneCount;
        if (doneCount === tasks.length && tasks.length > 0) {
          wf.workflow.status.state = 2; // COMPLETED
          wf.workflow.status.completed_at = now;
          this.emitEvent({
            timestamp: now,
            type: 'WORKFLOW_COMPLETED',
            workflow_id: wf.workflow.spec.id,
            term: this.term,
          });
        }
      });

      // 3. Update worker loads
      this.workers.forEach(w => {
        if (!w.active) {
          w.current_tasks = 0;
          w.running_tasks = 0;
          return;
        }
        let count = 0;
        this.workflows.forEach(wf => {
          wf.tasks.forEach(t => {
            if (t.status.assigned_worker === w.worker_id && t.status.state === TaskState.RUNNING) {
              count++;
            }
          });
        });
        w.current_tasks = count;
        w.running_tasks = count;
      });
    }, 1500);
  }

  async getClusterStatus(): Promise<ClusterStatus> {
    let queueDepth = 0;
    this.workflows.forEach(wf => {
      queueDepth += wf.tasks.filter(t => t.status.state === TaskState.READY).length;
    });

    return {
      node_id: this.currentLeader,
      is_leader: true,
      fencing_token: this.term,
      leader_key: `/orchestra/leader/election/${this.currentLeader}`,
      leader: {
        node_id: this.currentLeader,
        grpc_addr: `${this.currentLeader}:50051`,
        http_addr: `http://${this.currentLeader}:8080`,
        started_at: this.startTime,
      },
      term: this.term,
      nodes: this.orchestrators.map(o => ({
        ...o,
        http_addr: '',
        is_leader: o.node_id === this.currentLeader,
        role: o.node_id === this.currentLeader ? 'LEADER' : 'STANDBY',
      })),
      worker_count: this.workers.filter(w => w.active).length,
      workers: this.workers.map(w => ({
        worker_id: w.worker_id,
        capacity: w.capacity,
        current_tasks: w.current_tasks,
        running_tasks: w.running_tasks,
        active: w.active,
      })),
      queue_depth: queueDepth,
      total_workflows: this.workflows.length,
      recent_events: this.recentEvents,
      timestamp: Date.now(),
    };
  }

  async getWorkflows(): Promise<WorkflowWithTasks[]> {
    return this.workflows;
  }

  async getWorkflow(id: string): Promise<WorkflowWithTasks | null> {
    return this.workflows.find(w => w.workflow.spec.id === id) || null;
  }

  async submitWorkflow(_specStr?: string): Promise<{ workflow_id: string }> {
    const id = `wf-pipeline-${Math.floor(this.rng() * 9000 + 1000)}`;
    const newWf: WorkflowWithTasks = {
      workflow: {
        spec: { id, name: `Generated Pipeline ${id}`, failure_policy: 1, tasks: [] },
        status: { id, state: 1, created_at: Date.now(), completed_at: 0, total_tasks: 4, completed_tasks: 0, failed_tasks: 0 },
      },
      tasks: [
        { spec: { id: 'step-1', type: 'sleep', payload: 'sleep: 2s', timeout_seconds: 20, max_retries: 2, dependencies: [] }, status: { id: 'step-1', workflow_id: id, state: TaskState.READY, current_attempt: 0, assigned_worker: '', started_at: 0, completed_at: 0, error_message: '' } },
        { spec: { id: 'step-2a', type: 'sleep', payload: 'sleep: 3s', timeout_seconds: 20, max_retries: 2, dependencies: ['step-1'] }, status: { id: 'step-2a', workflow_id: id, state: TaskState.PENDING, current_attempt: 0, assigned_worker: '', started_at: 0, completed_at: 0, error_message: '' } },
        { spec: { id: 'step-2b', type: 'sleep', payload: 'sleep: 3s', timeout_seconds: 20, max_retries: 2, dependencies: ['step-1'] }, status: { id: 'step-2b', workflow_id: id, state: TaskState.PENDING, current_attempt: 0, assigned_worker: '', started_at: 0, completed_at: 0, error_message: '' } },
        { spec: { id: 'step-3', type: 'sleep', payload: 'sleep: 2s', timeout_seconds: 20, max_retries: 2, dependencies: ['step-2a', 'step-2b'] }, status: { id: 'step-3', workflow_id: id, state: TaskState.PENDING, current_attempt: 0, assigned_worker: '', started_at: 0, completed_at: 0, error_message: '' } },
      ],
    };

    this.workflows.unshift(newWf);
    this.emitEvent({
      timestamp: Date.now(),
      type: 'WORKFLOW_SUBMITTED',
      workflow_id: id,
      term: this.term,
    });

    return { workflow_id: id };
  }

  async cancelWorkflow(id: string): Promise<void> {
    const wf = this.workflows.find(w => w.workflow.spec.id === id);
    if (wf) {
      wf.workflow.status.state = 3; // CANCELLED
      wf.tasks.forEach(t => {
        if (t.status.state !== TaskState.COMPLETED) {
          t.status.state = TaskState.CANCELLED;
        }
      });
      this.emitEvent({
        timestamp: Date.now(),
        type: 'WORKFLOW_CANCELLED',
        workflow_id: id,
        term: this.term,
      });
    }
  }

  async killLeader(): Promise<ChaosResult> {
    const oldLeader = this.currentLeader;
    this.term++;
    // Elect standby
    this.currentLeader = oldLeader === 'orchestrator-1' ? 'orchestrator-2' : oldLeader === 'orchestrator-2' ? 'orchestrator-3' : 'orchestrator-1';

    this.emitEvent({
      timestamp: Date.now(),
      type: 'LEADER_FAILOVER',
      old_leader: oldLeader,
      new_leader: this.currentLeader,
      term: this.term,
    });

    return {
      status: 'leader failover simulated',
      leader_id: oldLeader,
      new_leader: this.currentLeader,
      term: this.term,
      killed_by_docker: false,
      timestamp: Date.now(),
    };
  }

  async killWorker(workerId?: string): Promise<ChaosResult> {
    const target = workerId || (this.workers.find(w => w.active)?.worker_id || 'worker-3');
    const w = this.workers.find(worker => worker.worker_id === target);
    if (w) {
      w.active = false;
      w.current_tasks = 0;
      w.running_tasks = 0;
    }

    // Migrate running tasks from dead worker to healthy worker (e.g. worker-1)
    const healthyWorker = this.workers.find(worker => worker.active && worker.worker_id !== target) || this.workers[0];
    this.workflows.forEach(wf => {
      wf.tasks.forEach(t => {
        if (t.status.assigned_worker === target && t.status.state === TaskState.RUNNING) {
          // Task re-queued and migrated
          t.status.assigned_worker = healthyWorker.worker_id;
          t.status.current_attempt = (t.status.current_attempt || 1) + 1;
          this.emitEvent({
            timestamp: Date.now(),
            type: 'TASK_MIGRATED',
            workflow_id: wf.workflow.spec.id,
            task_id: t.spec.id,
            old_state: 'RUNNING',
            new_state: 'RUNNING',
            worker_id: healthyWorker.worker_id,
            term: this.term,
          });
        }
      });
    });

    this.emitEvent({
      timestamp: Date.now(),
      type: 'WORKER_KILLED',
      worker_id: target,
      term: this.term,
    });

    // Auto-revive worker after 15 seconds to simulate rolling recovery
    setTimeout(() => {
      if (w) {
        w.active = true;
        this.emitEvent({
          timestamp: Date.now(),
          type: 'WORKER_RECOVERED',
          worker_id: target,
          term: this.term,
        });
      }
    }, 15000);

    return {
      status: 'worker halted and tasks migrated',
      worker_id: target,
      killed_by_docker: false,
      timestamp: Date.now(),
    };
  }

  async pauseLeader(): Promise<ChaosResult> {
    return this.killLeader();
  }

  async restartEtcdNode(nodeId: string = 'etcd-2'): Promise<ChaosResult> {
    this.emitEvent({
      timestamp: Date.now(),
      type: 'ETCD_QUORUM_DEGRADED',
      old_leader: nodeId,
      term: this.term,
    });

    setTimeout(() => {
      this.emitEvent({
        timestamp: Date.now(),
        type: 'ETCD_QUORUM_RESTORED',
        old_leader: nodeId,
        term: this.term,
      });
    }, 2000);

    return {
      status: `etcd follower ${nodeId} restarted`,
      node_id: nodeId,
      term: this.term,
      killed_by_docker: false,
      timestamp: Date.now(),
    };
  }

  subscribeEvents(onEvent: (event: EventRecord) => void): () => void {
    this.subscribers.add(onEvent);
    return () => this.subscribers.delete(onEvent);
  }
}
