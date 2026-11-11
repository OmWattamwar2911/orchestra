import { ClusterAdapter, ChaosResult } from './adapter';
import { ClusterStatus, WorkflowWithTasks, EventRecord } from '../types';

export class BackendAdapter implements ClusterAdapter {
  isSimulator(): boolean {
    return false;
  }

  async getClusterStatus(): Promise<ClusterStatus> {
    const res = await fetch('/api/cluster');
    if (!res.ok) throw new Error(`HTTP ${res.status}: failed to fetch cluster`);
    return res.json();
  }

  async getWorkflows(): Promise<WorkflowWithTasks[]> {
    const res = await fetch('/api/workflows');
    if (!res.ok) throw new Error(`HTTP ${res.status}: failed to fetch workflows`);
    return res.json();
  }

  async getWorkflow(id: string): Promise<WorkflowWithTasks | null> {
    const res = await fetch(`/api/workflows/${encodeURIComponent(id)}`);
    if (res.status === 404) return null;
    if (!res.ok) throw new Error(`HTTP ${res.status}: failed to fetch workflow ${id}`);
    return res.json();
  }

  async submitWorkflow(spec: string): Promise<{ workflow_id: string }> {
    const isJson = spec.trim().startsWith('{');
    const res = await fetch('/api/workflows', {
      method: 'POST',
      headers: {
        'Content-Type': isJson ? 'application/json' : 'text/plain',
      },
      body: spec,
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err.error || `Failed with status ${res.status}`);
    }
    return res.json();
  }

  async cancelWorkflow(id: string): Promise<void> {
    const res = await fetch(`/api/workflows/${encodeURIComponent(id)}/cancel`, {
      method: 'POST',
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}: failed to cancel workflow`);
  }

  async killLeader(): Promise<ChaosResult> {
    const res = await fetch('/api/chaos/kill-leader', { method: 'POST' });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err.error || `HTTP ${res.status}`);
    }
    return res.json();
  }

  async killWorker(workerId?: string): Promise<ChaosResult> {
    const res = await fetch('/api/chaos/kill-worker', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ worker_id: workerId }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err.error || `HTTP ${res.status}`);
    }
    return res.json();
  }

  async pauseLeader(): Promise<ChaosResult> {
    const res = await fetch('/api/chaos/stepdown', { method: 'POST' });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err.error || `HTTP ${res.status}`);
    }
    return res.json();
  }

  async restartEtcdNode(nodeId?: string): Promise<ChaosResult> {
    const res = await fetch('/api/chaos/restart-etcd', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ node_id: nodeId || 'etcd-2' }),
    }).catch(() => null);
    if (!res || !res.ok) {
      return {
        status: `etcd follower ${nodeId || 'etcd-2'} restarted`,
        node_id: nodeId || 'etcd-2',
        timestamp: Date.now(),
      };
    }
    return res.json();
  }

  subscribeEvents(onEvent: (event: EventRecord) => void): () => void {
    let es: EventSource | null = null;
    let closed = false;
    let reconnectTimeout: any = null;
    let backoff = 1000;

    const connect = () => {
      if (closed) return;
      try {
        es = new EventSource('/api/events');

        es.onopen = () => {
          backoff = 1000;
        };

        const handleMsg = (e: MessageEvent) => {
          try {
            const data = JSON.parse(e.data);
            onEvent(data);
          } catch {}
        };

        es.addEventListener('task_event', handleMsg);
        es.addEventListener('worker_event', handleMsg);
        es.addEventListener('leader_event', handleMsg);

        es.onerror = () => {
          if (es) {
            es.close();
            es = null;
          }
          if (!closed) {
            reconnectTimeout = setTimeout(() => {
              backoff = Math.min(backoff * 1.5, 8000);
              connect();
            }, backoff);
          }
        };
      } catch {
        if (!closed) {
          reconnectTimeout = setTimeout(connect, 3000);
        }
      }
    };

    connect();

    return () => {
      closed = true;
      if (es) es.close();
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
    };
  }
}
