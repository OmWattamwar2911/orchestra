import React, { createContext, useContext, useState, useEffect, useCallback, useRef } from 'react';
import { ClusterStatus, WorkflowWithTasks, EventRecord } from '../types';
import { clusterClient } from '../data/client';
import { useEventStream, SSEConnectionStatus } from '../data/useEventStream';
import { fallbackCluster, fallbackWorkflows } from '../data/initialData';

interface ClusterContextValue {
  cluster: ClusterStatus | null;
  workflows: WorkflowWithTasks[];
  events: EventRecord[];
  sseStatus: SSEConnectionStatus;
  isSimulator: boolean;
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  submitWorkflow: (spec: string) => Promise<{ workflow_id: string }>;
  cancelWorkflow: (id: string) => Promise<void>;
  clearEvents: () => void;
}

const ClusterContext = createContext<ClusterContextValue | null>(null);

export const ClusterProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [cluster, setCluster] = useState<ClusterStatus | null>(fallbackCluster);
  const [workflows, setWorkflows] = useState<WorkflowWithTasks[]>(fallbackWorkflows);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isSimulator, setIsSimulator] = useState(() => clusterClient.isSimulator());

  const refresh = useCallback(async () => {
    try {
      const [clusterData, workflowsData] = await Promise.all([
        clusterClient.getClusterStatus(),
        clusterClient.getWorkflows(),
      ]);
      setCluster(clusterData);
      setWorkflows(workflowsData);
      setIsSimulator(clusterClient.isSimulator());
      setError(null);
    } catch (err: any) {
      setError(err?.message || 'Failed to fetch cluster status');
      setIsSimulator(clusterClient.isSimulator());
    } finally {
      setLoading(false);
    }
  }, []);

  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;

  const { status: sseStatus, events, clearEvents, setEvents } = useEventStream({
    onEvent: () => {
      // Trigger refresh when events arrive
      refreshRef.current();
    },
  });

  // Initial load & periodic polling as fallback
  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 3000);
    return () => clearInterval(interval);
  }, [refresh]);

  // If initial cluster has recent_events, seed events
  useEffect(() => {
    if (cluster?.recent_events && cluster.recent_events.length > 0 && events.length === 0) {
      setEvents(cluster.recent_events);
    }
  }, [cluster, events.length, setEvents]);

  const submitWorkflow = useCallback(async (spec: string) => {
    const res = await clusterClient.submitWorkflow(spec);
    await refresh();
    return res;
  }, [refresh]);

  const cancelWorkflow = useCallback(async (id: string) => {
    await clusterClient.cancelWorkflow(id);
    await refresh();
  }, [refresh]);

  return (
    <ClusterContext.Provider
      value={{
        cluster,
        workflows,
        events,
        sseStatus,
        isSimulator,
        loading,
        error,
        refresh,
        submitWorkflow,
        cancelWorkflow,
        clearEvents,
      }}
    >
      {children}
    </ClusterContext.Provider>
  );
};

export function useCluster() {
  const context = useContext(ClusterContext);
  if (!context) {
    throw new Error('useCluster must be used within a ClusterProvider');
  }
  return context;
}
