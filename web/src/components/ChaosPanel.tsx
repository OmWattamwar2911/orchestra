import React, { useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { 
  ShieldAlert, 
  Cpu, 
  RefreshCw, 
  X, 
  AlertTriangle, 
  Skull,
  Radio
} from 'lucide-react';
import { ClusterStatus } from '../types';
import { isChaosFlagEnabled } from './Header';

interface ChaosPanelProps {
  isOpen: boolean;
  onClose: () => void;
  cluster: ClusterStatus | null;
  onActionComplete: () => void;
}

const sanitizeNodeName = (id?: string): string => {
  if (!id) return 'Unknown';
  return id.replace(/[-_]/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
};

const sanitizeWorkerName = (id: string): string => {
  return id.replace(/^distributed-scheduler-/, '').replace(/[-_]/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
};

export const ChaosPanel: React.FC<ChaosPanelProps> = ({
  isOpen,
  onClose,
  cluster,
  onActionComplete,
}) => {
  const [selectedWorker, setSelectedWorker] = useState<string>('');
  const [loadingAction, setLoadingAction] = useState<string | null>(null);
  const [chaosLogs, setChaosLogs] = useState<{ time: string; text: string; ok: boolean }[]>([]);

  if (!isChaosFlagEnabled()) return null;

  const addLog = (text: string, ok: boolean) => {
    const time = new Date().toLocaleTimeString([], { hour12: false });
    setChaosLogs(prev => [{ time, text, ok }, ...prev.slice(0, 20)]);
  };

  const killWorker = async () => {
    setLoadingAction('worker');
    try {
      const res = await fetch('/api/chaos/kill-worker', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ worker_id: selectedWorker || undefined }),
      });
      const data = await res.json();
      if (res.ok) {
        addLog(`Worker ${sanitizeWorkerName(data.worker_id)} terminated (lease revoked in cluster)`, true);
      } else {
        addLog(`Failed to terminate worker: ${data.error || res.statusText}`, false);
      }
      onActionComplete();
    } catch (err: any) {
      addLog(`Error executing worker termination: ${err.message}`, false);
    } finally {
      setLoadingAction(null);
    }
  };

  const killLeader = async () => {
    setLoadingAction('leader');
    try {
      const res = await fetch('/api/chaos/kill-leader', {
        method: 'POST',
      });
      const data = await res.json();
      if (res.ok) {
        addLog(`Active leader ${sanitizeNodeName(data.leader_id)} killed. Failover initiated (Term ${data.term} -> new)`, true);
      } else {
        addLog(`Failed to kill leader: ${data.error || res.statusText}`, false);
      }
      onActionComplete();
    } catch (err: any) {
      addLog(`Error triggering leader kill: ${err.message}`, false);
    } finally {
      setLoadingAction(null);
    }
  };

  const stepdownLeader = async () => {
    setLoadingAction('stepdown');
    try {
      const res = await fetch('/api/chaos/stepdown', {
        method: 'POST',
      });
      const data = await res.json();
      if (res.ok) {
        addLog('Active leader resigned voluntarily via election session', true);
      } else {
        addLog(`Resignation rejected: ${data.error || res.statusText}`, false);
      }
      onActionComplete();
    } catch (err: any) {
      addLog(`Error during stepdown: ${err.message}`, false);
    } finally {
      setLoadingAction(null);
    }
  };

  const workers = cluster?.workers || [];
  const rawLeader = cluster?.leader?.node_id || (cluster?.is_leader ? cluster?.node_id : '');
  const currentLeader = rawLeader ? sanitizeNodeName(rawLeader) : 'Unknown';

  return (
    <AnimatePresence>
      {isOpen && (
        <motion.aside
          initial={false}
          animate={{ x: 0, opacity: 1 }}
          exit={{ x: 420, opacity: 0 }}
          transition={{ duration: 0.15, ease: 'easeOut' }}
          className="fixed top-0 right-0 bottom-0 z-50 flex w-full sm:w-[420px] flex-col border-l border-danger-border bg-surface shadow-2xl overflow-y-auto"
          aria-label="Chaos engineering control panel"
        >
          {/* Header */}
          <div className="flex items-center justify-between border-b border-border-default bg-surface-elevated p-4">
            <div className="flex items-center gap-2.5">
              <ShieldAlert className="h-5 w-5 text-danger-text" aria-hidden="true" />
              <div>
                <h2 className="text-sm font-semibold text-text-primary">
                  Chaos engineering
                </h2>
                <span className="text-xs text-text-muted">
                  Development fault injection
                </span>
              </div>
            </div>

            <button
              type="button"
              onClick={onClose}
              className="rounded p-1 text-text-muted hover:bg-surface hover:text-text-primary focus-visible:ring-2 focus-visible:ring-accent-focus focus-visible:outline-none"
              aria-label="Close chaos panel"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>

          {/* Warning Banner */}
          <div className="flex items-start gap-2.5 border-b border-warning-border bg-warning-bg p-3.5 text-xs text-warning-text">
            <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" aria-hidden="true" />
            <span>
              Fault injections terminate active container processes and trigger consensus election failovers.
            </span>
          </div>

          {/* Action Sections */}
          <div className="flex flex-col gap-4 p-4">
            {/* Leader Fault Injection */}
            <section aria-label="Leader fault injection" className="rounded-lg border border-border-default bg-surface-subtle p-4">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 mb-2">
                <span className="text-xs font-semibold text-text-primary">Leader chaos</span>
                <span className="font-mono text-xs text-accent-text truncate">Current: {currentLeader}</span>
              </div>
              <p className="text-xs text-text-muted mb-3">
                Terminates the active leader orchestrator container or steps down leadership to trigger consensus failover.
              </p>
              <div className="flex flex-col sm:flex-row gap-2">
                <button
                  type="button"
                  onClick={killLeader}
                  disabled={loadingAction !== null}
                  className="flex-1 flex items-center justify-center gap-1.5 rounded-md border border-danger-border bg-danger-bg px-3 py-2 text-xs font-semibold text-danger-text hover:bg-danger-solid hover:text-text-primary disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-accent-focus focus-visible:outline-none transition-colors"
                >
                  <Skull className="h-3.5 w-3.5" aria-hidden="true" />
                  <span>{loadingAction === 'leader' ? 'Killing leader...' : 'Kill leader'}</span>
                </button>

                <button
                  type="button"
                  onClick={stepdownLeader}
                  disabled={loadingAction !== null || !cluster?.is_leader}
                  className="flex items-center justify-center gap-1.5 rounded-md border border-border-default bg-surface px-3 py-2 text-xs font-medium text-text-secondary hover:bg-surface-elevated hover:text-text-primary disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-accent-focus focus-visible:outline-none transition-colors"
                >
                  <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
                  <span>Step down</span>
                </button>
              </div>
            </section>

            {/* Worker Fault Injection */}
            <section aria-label="Worker fault injection" className="rounded-lg border border-border-default bg-surface-subtle p-4">
              <div className="text-xs font-semibold text-text-primary mb-1">
                Worker chaos
              </div>
              <p className="text-xs text-text-muted mb-3">
                Revokes worker liveness lease to test in-flight task re-assignment to remaining healthy workers.
              </p>

              <div className="flex flex-col gap-2.5">
                <label htmlFor="worker-picker" className="sr-only">Select target worker</label>
                <select
                  id="worker-picker"
                  value={selectedWorker}
                  onChange={e => setSelectedWorker(e.target.value)}
                  className="rounded-md border border-border-default bg-surface px-3 py-1.5 font-mono text-xs text-text-primary focus-visible:ring-2 focus-visible:ring-accent-focus focus-visible:outline-none"
                >
                  <option value="">(Random active worker)</option>
                  {workers.map(w => (
                    <option key={w.worker_id} value={w.worker_id}>
                      {sanitizeWorkerName(w.worker_id)}
                    </option>
                  ))}
                </select>

                <button
                  type="button"
                  onClick={killWorker}
                  disabled={loadingAction !== null}
                  className="flex items-center justify-center gap-1.5 rounded-md border border-warning-border bg-warning-bg px-3 py-2 text-xs font-semibold text-warning-text hover:bg-warning-solid hover:text-text-inverse disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-accent-focus focus-visible:outline-none transition-colors"
                >
                  <Cpu className="h-3.5 w-3.5" aria-hidden="true" />
                  <span>{loadingAction === 'worker' ? 'Killing worker...' : 'Kill worker'}</span>
                </button>
              </div>
            </section>
          </div>

          {/* Activity Log */}
          <section aria-label="Chaos execution logs" className="flex flex-1 flex-col p-4 border-t border-border-subtle bg-surface-elevated/20">
            <div className="flex items-center gap-2 mb-2 text-xs font-semibold text-text-secondary">
              <Radio className="h-3.5 w-3.5 text-accent-text" aria-hidden="true" />
              <span>Fault activity log</span>
            </div>
            <div className="flex-1 overflow-y-auto rounded-md border border-border-subtle bg-canvas p-3 font-mono text-xs">
              {chaosLogs.length > 0 ? (
                chaosLogs.map((log, idx) => (
                  <div key={idx} className="flex gap-2 py-1 leading-snug">
                    <span className="text-text-muted shrink-0">{log.time}</span>
                    <span className={log.ok ? 'text-success-text' : 'text-danger-text'}>
                      {log.text}
                    </span>
                  </div>
                ))
              ) : (
                <div className="text-text-muted text-center py-6">
                  No chaos actions executed yet. Trigger a fault above to observe cluster recovery.
                </div>
              )}
            </div>
          </section>
        </motion.aside>
      )}
    </AnimatePresence>
  );
};
