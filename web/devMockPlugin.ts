import type { Plugin } from 'vite';
import { load as yamlLoad } from 'js-yaml';
import type { IncomingMessage, ServerResponse } from 'http';

interface MockTask {
  spec: {
    id: string;
    type?: string;
    payload?: string;
    timeout_seconds?: number;
    max_retries?: number;
    dependencies?: string[];
  };
  status: {
    id: string;
    workflow_id: string;
    state: number; // 1: PENDING, 2: READY, 3: ASSIGNED, 4: RUNNING, 5: COMPLETED, etc.
    current_attempt: number;
    assigned_worker: string;
    started_at: number;
    completed_at: number;
    error_message: string;
  };
}

interface MockWorkflow {
  workflow: {
    spec: {
      id: string;
      name: string;
      failure_policy: number;
      tasks: any[];
    };
    status: {
      id: string;
      state: number; // 1: RUNNING, 2: SUCCEEDED, etc.
      created_at: number;
      completed_at: number;
      total_tasks: number;
      completed_tasks: number;
      failed_tasks: number;
    };
  };
  tasks: MockTask[];
}

export function devMockPlugin(): Plugin {
  let term = 1;
  let currentLeader = 'orchestrator-1';
  let queueDepth = 0;

  const orchestrators = [
    { node_id: 'orchestrator-1', addr: 'orchestrator1:50051', http_addr: 'http://orchestrator1:8080', is_leader: true, role: 'LEADER' as const, status: 'UP' },
    { node_id: 'orchestrator-2', addr: 'orchestrator2:50051', http_addr: 'http://orchestrator2:8080', is_leader: false, role: 'STANDBY' as const, status: 'UP' },
    { node_id: 'orchestrator-3', addr: 'orchestrator3:50051', http_addr: 'http://orchestrator3:8080', is_leader: false, role: 'STANDBY' as const, status: 'UP' },
  ];

  const workers = [
    { worker_id: 'distributed-scheduler-worker-1', capacity: 4, current_tasks: 1, running_tasks: 1, active: true },
    { worker_id: 'distributed-scheduler-worker-2', capacity: 4, current_tasks: 1, running_tasks: 1, active: true },
    { worker_id: 'distributed-scheduler-worker-3', capacity: 4, current_tasks: 0, running_tasks: 0, active: true },
    { worker_id: 'distributed-scheduler-worker-4', capacity: 4, current_tasks: 0, running_tasks: 0, active: true },
    { worker_id: 'distributed-scheduler-worker-5', capacity: 4, current_tasks: 0, running_tasks: 0, active: true },
  ];

  const recentEvents: any[] = [
    {
      timestamp: Date.now() - 15000,
      type: 'TASK_TRANSITION',
      workflow_id: 'diamond-pipeline-sample',
      task_id: 'fetch',
      old_state: 'READY',
      new_state: 'COMPLETED',
      worker_id: 'distributed-scheduler-worker-1',
      term: 1,
    },
    {
      timestamp: Date.now() - 10000,
      type: 'TASK_TRANSITION',
      workflow_id: 'diamond-pipeline-sample',
      task_id: 'transformA',
      old_state: 'READY',
      new_state: 'RUNNING',
      worker_id: 'distributed-scheduler-worker-1',
      term: 1,
    },
    {
      timestamp: Date.now() - 8000,
      type: 'TASK_TRANSITION',
      workflow_id: 'diamond-pipeline-sample',
      task_id: 'transformB',
      old_state: 'READY',
      new_state: 'RUNNING',
      worker_id: 'distributed-scheduler-worker-2',
      term: 1,
    },
  ];

  const workflows: MockWorkflow[] = [
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
          state: 1, // RUNNING
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
          status: { id: 'fetch', workflow_id: 'diamond-pipeline-sample', state: 5 /* COMPLETED */, current_attempt: 1, assigned_worker: 'distributed-scheduler-worker-1', started_at: Date.now() - 20000, completed_at: Date.now() - 15000, error_message: '' },
        },
        {
          spec: { id: 'transformA', type: 'sleep', payload: 'sleep: 3s', timeout_seconds: 20, max_retries: 2, dependencies: ['fetch'] },
          status: { id: 'transformA', workflow_id: 'diamond-pipeline-sample', state: 4 /* RUNNING */, current_attempt: 1, assigned_worker: 'distributed-scheduler-worker-1', started_at: Date.now() - 10000, completed_at: 0, error_message: '' },
        },
        {
          spec: { id: 'transformB', type: 'sleep', payload: 'sleep: 4s', timeout_seconds: 20, max_retries: 2, dependencies: ['fetch'] },
          status: { id: 'transformB', workflow_id: 'diamond-pipeline-sample', state: 4 /* RUNNING */, current_attempt: 1, assigned_worker: 'distributed-scheduler-worker-2', started_at: Date.now() - 8000, completed_at: 0, error_message: '' },
        },
        {
          spec: { id: 'merge', type: 'mock', payload: 'merge transformed chunks', timeout_seconds: 20, max_retries: 2, dependencies: ['transformA', 'transformB'] },
          status: { id: 'merge', workflow_id: 'diamond-pipeline-sample', state: 1 /* PENDING */, current_attempt: 0, assigned_worker: '', started_at: 0, completed_at: 0, error_message: '' },
        },
        {
          spec: { id: 'publish', type: 'mock', payload: 'publish aggregated result', timeout_seconds: 20, max_retries: 2, dependencies: ['merge'] },
          status: { id: 'publish', workflow_id: 'diamond-pipeline-sample', state: 1 /* PENDING */, current_attempt: 0, assigned_worker: '', started_at: 0, completed_at: 0, error_message: '' },
        },
      ],
    },
  ];

  const sseClients = new Set<ServerResponse>();

  function broadcastSSE(event: string, data: any) {
    const payload = `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;
    sseClients.forEach(res => {
      try {
        res.write(payload);
      } catch {
        sseClients.delete(res);
      }
    });
  }

  // Simulation loop: progress running tasks
  setInterval(() => {
    workflows.forEach(wf => {
      if (wf.workflow.status.state !== 1) return; // not running

      // Find any RUNNING tasks and complete them if ready
      let changed = false;
      const completedSet = new Set(wf.tasks.filter(t => t.status.state === 5).map(t => t.spec.id));

      wf.tasks.forEach(task => {
        if (task.status.state === 4 /* RUNNING */) {
          // 30% chance to complete on tick
          if (Math.random() > 0.4) {
            task.status.state = 5; // COMPLETED
            task.status.completed_at = Date.now();
            completedSet.add(task.spec.id);
            changed = true;

            const ev = {
              timestamp: Date.now(),
              type: 'TASK_COMPLETED',
              workflow_id: wf.workflow.spec.id,
              task_id: task.spec.id,
              old_state: 'RUNNING',
              new_state: 'COMPLETED',
              worker_id: task.status.assigned_worker,
              term,
            };
            recentEvents.unshift(ev);
            broadcastSSE('task_event', ev);
          }
        }
      });

      // Find PENDING tasks whose dependencies are met and move to READY -> RUNNING
      wf.tasks.forEach(task => {
        if (task.status.state === 1 /* PENDING */) {
          const deps = task.spec.dependencies || [];
          const allMet = deps.every(d => completedSet.has(d));
          if (allMet) {
            task.status.state = 4; // RUNNING
            task.status.current_attempt = 1;
            task.status.started_at = Date.now();
            const w = workers[Math.floor(Math.random() * workers.length)];
            task.status.assigned_worker = w.worker_id;
            changed = true;

            const ev = {
              timestamp: Date.now(),
              type: 'TASK_SCHEDULED',
              workflow_id: wf.workflow.spec.id,
              task_id: task.spec.id,
              old_state: 'READY',
              new_state: 'RUNNING',
              worker_id: w.worker_id,
              term,
            };
            recentEvents.unshift(ev);
            broadcastSSE('task_event', ev);
          }
        }
      });

      // Check if all tasks completed
      const allDone = wf.tasks.every(t => t.status.state === 5);
      if (allDone) {
        wf.workflow.status.state = 2; // SUCCEEDED
        wf.workflow.status.completed_at = Date.now();
        broadcastSSE('task_event', {
          timestamp: Date.now(),
          type: 'WORKFLOW_SUCCEEDED',
          workflow_id: wf.workflow.spec.id,
          term,
        });
      }

      if (changed) {
        wf.workflow.status.completed_tasks = wf.tasks.filter(t => t.status.state === 5).length;
      }
    });

    // Update worker loads
    workers.forEach(w => {
      let load = 0;
      workflows.forEach(wf => {
        wf.tasks.forEach(t => {
          if (t.status.assigned_worker === w.worker_id && t.status.state === 4) {
            load++;
          }
        });
      });
      w.current_tasks = load;
      w.running_tasks = load;
    });
  }, 2500);

  return {
    name: 'dev-mock-backend',
    configureServer(server) {
      server.middlewares.use((req: IncomingMessage, res: ServerResponse, next) => {
        const url = req.url || '';

        // Check if request is targeting /api
        if (!url.startsWith('/api')) {
          return next();
        }

        // GET /api/cluster
        if (url === '/api/cluster' && req.method === 'GET') {
          res.setHeader('Content-Type', 'application/json');
          const data = {
            node_id: currentLeader,
            is_leader: true,
            fencing_token: term,
            leader_key: `/orchestra/leader/election/${currentLeader}`,
            leader: {
              node_id: currentLeader,
              grpc_addr: `${currentLeader}:50051`,
              http_addr: `http://${currentLeader}:8080`,
              started_at: Date.now() - 60000,
            },
            term,
            nodes: orchestrators,
            worker_count: workers.length,
            workers,
            queue_depth: queueDepth,
            total_workflows: workflows.length,
            recent_events: recentEvents.slice(0, 30),
            timestamp: Date.now(),
          };
          res.end(JSON.stringify(data));
          return;
        }

        // GET /api/workflows
        if (url === '/api/workflows' && req.method === 'GET') {
          res.setHeader('Content-Type', 'application/json');
          res.end(JSON.stringify(workflows));
          return;
        }

        // GET /api/workflows/:id
        if (url.startsWith('/api/workflows/') && req.method === 'GET') {
          const id = url.replace('/api/workflows/', '').split('?')[0];
          const found = workflows.find(w => w.workflow.spec.id === id);
          if (found) {
            res.setHeader('Content-Type', 'application/json');
            res.end(JSON.stringify(found));
          } else {
            res.statusCode = 404;
            res.end(JSON.stringify({ error: 'workflow not found' }));
          }
          return;
        }

        // POST /api/workflows/:id/cancel
        if (url.startsWith('/api/workflows/') && url.endsWith('/cancel') && req.method === 'POST') {
          const parts = url.split('/');
          const id = parts[3];
          const found = workflows.find(w => w.workflow.spec.id === id);
          if (found) {
            found.workflow.status.state = 4; // CANCELLED
            found.tasks.forEach(t => {
              if (t.status.state !== 5) {
                t.status.state = 8; // CANCELLED
              }
            });
            broadcastSSE('task_event', {
              timestamp: Date.now(),
              type: 'WORKFLOW_CANCELLED',
              workflow_id: id,
              term,
            });
            res.setHeader('Content-Type', 'application/json');
            res.end(JSON.stringify({ status: 'cancelled', workflow_id: id }));
          } else {
            res.statusCode = 404;
            res.end(JSON.stringify({ error: 'not found' }));
          }
          return;
        }

        // POST /api/workflows (Submit)
        if (url === '/api/workflows' && req.method === 'POST') {
          let body = '';
          req.on('data', chunk => {
            body += chunk;
          });
          req.on('end', () => {
            try {
              let parsed: any;
              const trimmed = body.trim();
              if (trimmed.startsWith('{')) {
                parsed = JSON.parse(trimmed);
              } else {
                parsed = yamlLoad(trimmed);
              }

              const wfId = parsed.id || `wf-${Date.now()}`;
              const taskSpecs = parsed.tasks || [];

              const newWf: MockWorkflow = {
                workflow: {
                  spec: {
                    id: wfId,
                    name: parsed.name || wfId,
                    failure_policy: parsed.failure_policy === 'CONTINUE' ? 2 : 1,
                    tasks: taskSpecs,
                  },
                  status: {
                    id: wfId,
                    state: 1, // RUNNING
                    created_at: Date.now(),
                    completed_at: 0,
                    total_tasks: taskSpecs.length,
                    completed_tasks: 0,
                    failed_tasks: 0,
                  },
                },
                tasks: taskSpecs.map((t: any) => {
                  const hasDeps = t.dependencies && t.dependencies.length > 0;
                  return {
                    spec: {
                      id: t.id,
                      type: t.type || 'mock',
                      payload: t.payload || '',
                      timeout_seconds: t.timeout_seconds || 30,
                      max_retries: t.max_retries || 2,
                      dependencies: t.dependencies || [],
                    },
                    status: {
                      id: t.id,
                      workflow_id: wfId,
                      state: hasDeps ? 1 /* PENDING */ : 4 /* RUNNING immediately if root */,
                      current_attempt: hasDeps ? 0 : 1,
                      assigned_worker: hasDeps ? '' : workers[0].worker_id,
                      started_at: hasDeps ? 0 : Date.now(),
                      completed_at: 0,
                      error_message: '',
                    },
                  };
                }),
              };

              workflows.unshift(newWf);

              const ev = {
                timestamp: Date.now(),
                type: 'WORKFLOW_SUBMITTED',
                workflow_id: wfId,
                term,
              };
              recentEvents.unshift(ev);
              broadcastSSE('task_event', ev);

              res.setHeader('Content-Type', 'application/json');
              res.end(JSON.stringify({ status: 'submitted', workflow_id: wfId }));
            } catch (err: any) {
              res.statusCode = 400;
              res.end(JSON.stringify({ error: err.message }));
            }
          });
          return;
        }

        // GET /api/events (SSE)
        if (url === '/api/events' && req.method === 'GET') {
          res.writeHead(200, {
            'Content-Type': 'text/event-stream',
            'Cache-Control': 'no-cache',
            Connection: 'keep-alive',
            'Access-Control-Allow-Origin': '*',
          });

          // Send initial connected event
          const initPayload = JSON.stringify({
            node_id: currentLeader,
            is_leader: true,
            connected: true,
            timestamp: Date.now(),
          });
          res.write(`event: connected\ndata: ${initPayload}\n\n`);

          sseClients.add(res);

          req.on('close', () => {
            sseClients.delete(res);
          });
          return;
        }

        // POST /api/chaos/kill-leader
        if (url === '/api/chaos/kill-leader' && req.method === 'POST') {
          const oldLeader = currentLeader;
          term++;
          currentLeader = currentLeader === 'orchestrator-1' ? 'orchestrator-2' : currentLeader === 'orchestrator-2' ? 'orchestrator-3' : 'orchestrator-1';

          orchestrators.forEach(n => {
            n.is_leader = n.node_id === currentLeader;
            n.role = n.is_leader ? 'LEADER' : 'STANDBY';
          });

          const ev = {
            timestamp: Date.now(),
            type: 'LEADER_ELECTION',
            old_leader: oldLeader,
            new_leader: currentLeader,
            term,
          };
          recentEvents.unshift(ev);

          // Close all existing SSE connections to simulate disconnection & reconnect
          sseClients.forEach(c => {
            try {
              c.write(`event: leader_event\ndata: ${JSON.stringify(ev)}\n\n`);
              c.end();
            } catch {}
          });
          sseClients.clear();

          res.setHeader('Content-Type', 'application/json');
          res.end(JSON.stringify({
            status: 'leader killed',
            leader_id: oldLeader,
            new_leader: currentLeader,
            term,
            killed_by_docker: true,
            timestamp: Date.now(),
          }));
          return;
        }

        // POST /api/chaos/kill-worker
        if (url === '/api/chaos/kill-worker' && req.method === 'POST') {
          let body = '';
          req.on('data', chunk => body += chunk);
          req.on('end', () => {
            let targetId = '';
            try {
              const b = JSON.parse(body || '{}');
              targetId = b.worker_id;
            } catch {}
            if (!targetId) targetId = workers[0].worker_id;

            const ev = {
              timestamp: Date.now(),
              type: 'WORKER_KILLED',
              worker_id: targetId,
              term,
            };
            recentEvents.unshift(ev);
            broadcastSSE('worker_event', ev);

            res.setHeader('Content-Type', 'application/json');
            res.end(JSON.stringify({
              status: 'worker killed',
              worker_id: targetId,
              killed_by_docker: true,
              timestamp: Date.now(),
            }));
          });
          return;
        }

        // POST /api/chaos/stepdown
        if (url === '/api/chaos/stepdown' && req.method === 'POST') {
          term++;
          currentLeader = currentLeader === 'orchestrator-1' ? 'orchestrator-2' : 'orchestrator-1';
          orchestrators.forEach(n => {
            n.is_leader = n.node_id === currentLeader;
            n.role = n.is_leader ? 'LEADER' : 'STANDBY';
          });
          res.setHeader('Content-Type', 'application/json');
          res.end(JSON.stringify({ status: 'stepped_down', new_leader: currentLeader, term }));
          return;
        }

        next();
      });
    },
  };
}
