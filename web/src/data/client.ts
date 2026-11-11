import { ClusterAdapter, ChaosResult } from './adapter';
import { BackendAdapter } from './backendAdapter';
import { SimulatorAdapter } from './simulatorAdapter';
import { ClusterStatus, WorkflowWithTasks, EventRecord } from '../types';

export class SmartClusterClient implements ClusterAdapter {
  private backend = new BackendAdapter();
  private simulator = new SimulatorAdapter();
  private forceSimulator: boolean;

  constructor() {
    this.forceSimulator =
      import.meta.env.VITE_DEMO === 'true' ||
      (typeof window !== 'undefined' && window.location.search.includes('demo=true'));
  }

  isSimulator(): boolean {
    return this.forceSimulator;
  }

  setDemoMode(enabled: boolean) {
    this.forceSimulator = enabled;
  }

  async getClusterStatus(): Promise<ClusterStatus> {
    if (this.forceSimulator) {
      return this.simulator.getClusterStatus();
    }
    try {
      return await this.backend.getClusterStatus();
    } catch {
      this.forceSimulator = true;
      return this.simulator.getClusterStatus();
    }
  }

  async getWorkflows(): Promise<WorkflowWithTasks[]> {
    if (this.forceSimulator) {
      return this.simulator.getWorkflows();
    }
    try {
      return await this.backend.getWorkflows();
    } catch {
      this.forceSimulator = true;
      return this.simulator.getWorkflows();
    }
  }

  async getWorkflow(id: string): Promise<WorkflowWithTasks | null> {
    if (this.forceSimulator) {
      return this.simulator.getWorkflow(id);
    }
    try {
      return await this.backend.getWorkflow(id);
    } catch {
      this.forceSimulator = true;
      return this.simulator.getWorkflow(id);
    }
  }

  async submitWorkflow(spec: string): Promise<{ workflow_id: string }> {
    if (this.forceSimulator) {
      return this.simulator.submitWorkflow(spec);
    }
    try {
      return await this.backend.submitWorkflow(spec);
    } catch {
      this.forceSimulator = true;
      return this.simulator.submitWorkflow(spec);
    }
  }

  async cancelWorkflow(id: string): Promise<void> {
    if (this.forceSimulator) {
      return this.simulator.cancelWorkflow(id);
    }
    try {
      return await this.backend.cancelWorkflow(id);
    } catch {
      this.forceSimulator = true;
      return this.simulator.cancelWorkflow(id);
    }
  }

  async killLeader(): Promise<ChaosResult> {
    if (this.forceSimulator) {
      return this.simulator.killLeader();
    }
    try {
      return await this.backend.killLeader();
    } catch {
      this.forceSimulator = true;
      return this.simulator.killLeader();
    }
  }

  async killWorker(workerId?: string): Promise<ChaosResult> {
    if (this.forceSimulator) {
      return this.simulator.killWorker(workerId);
    }
    try {
      return await this.backend.killWorker(workerId);
    } catch {
      this.forceSimulator = true;
      return this.simulator.killWorker(workerId);
    }
  }

  async pauseLeader(): Promise<ChaosResult> {
    if (this.forceSimulator) {
      return this.simulator.pauseLeader();
    }
    try {
      return await this.backend.pauseLeader();
    } catch {
      this.forceSimulator = true;
      return this.simulator.pauseLeader();
    }
  }

  async restartEtcdNode(nodeId?: string): Promise<ChaosResult> {
    if (this.forceSimulator) {
      return this.simulator.restartEtcdNode(nodeId);
    }
    try {
      return await this.backend.restartEtcdNode(nodeId);
    } catch {
      this.forceSimulator = true;
      return this.simulator.restartEtcdNode(nodeId);
    }
  }

  subscribeEvents(onEvent: (event: EventRecord) => void): () => void {
    if (this.forceSimulator) {
      return this.simulator.subscribeEvents(onEvent);
    }
    const unsubSim = this.simulator.subscribeEvents(onEvent);
    const unsubBackend = this.backend.subscribeEvents(onEvent);
    return () => {
      unsubSim();
      unsubBackend();
    };
  }
}

export const clusterClient = new SmartClusterClient();
