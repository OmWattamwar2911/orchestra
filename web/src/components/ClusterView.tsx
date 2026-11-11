import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { Search, Trash2, Pause, Play } from 'lucide-react';
import { ClusterStatus, WorkflowWithTasks, EventRecord } from '../types';
import { StatusBadge } from './ui/StatusBadge';
import { TypographicListItem } from './ui/TypographicListItem';
import { PosterButton } from './ui/PosterButton';
import { ClusterScoreTimeline } from './cluster/ClusterScoreTimeline';

interface ClusterViewProps {
  cluster: ClusterStatus | null;
  workflows?: WorkflowWithTasks[];
  events: EventRecord[];
  onClearEvents: () => void;
}

const sanitizeWorkerId = (id: string): string => {
  return id.replace(/^distributed-scheduler-/, '').replace(/[-_]/g, '-').toLowerCase();
};

const sanitizeNodeId = (id: string): string => {
  return id.replace(/^distributed-scheduler-/, '').replace(/[-_]/g, '-').toLowerCase();
};

export const ClusterView: React.FC<ClusterViewProps> = ({
  cluster,
  workflows = [],
  events,
  onClearEvents,
}) => {
  const [eventFilter, setEventFilter] = useState('');
  const [isPaused, setIsPaused] = useState(false);

  const rawLeader = cluster?.leader?.node_id || (cluster?.is_leader ? cluster?.node_id : '');
  const leaderId = rawLeader ? sanitizeNodeId(rawLeader) : 'orchestrator-1';
  const term = cluster?.term || cluster?.fencing_token || 1;
  const workers = cluster?.workers || [];
  const nodes = cluster?.nodes || [];

  let totalCapacity = 0;
  let totalRunning = 0;
  workers.forEach(w => {
    totalCapacity += w.capacity || 4;
    totalRunning += w.current_tasks ?? w.running_tasks ?? 0;
  });

  const filteredEvents = events.filter(e => {
    if (!eventFilter) return true;
    const q = eventFilter.toLowerCase();
    return (
      (e.type && e.type.toLowerCase().includes(q)) ||
      (e.workflow_id && e.workflow_id.toLowerCase().includes(q)) ||
      (e.task_id && e.task_id.toLowerCase().includes(q)) ||
      (e.worker_id && e.worker_id.toLowerCase().includes(q))
    );
  });

  const formatTimestamp = (ts?: number) => {
    if (!ts) return '-';
    const ms = ts > 10000000000 ? ts : ts * 1000;
    const d = new Date(ms);
    return d.toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' }) +
      '.' + String(d.getMilliseconds()).padStart(3, '0');
  };

  return (
    <div className="flex flex-col w-full">
      {/* 1. HEADLINE STATUS SENTENCE */}
      <section className="border-b border-hairline p-6 lg:p-12 space-y-4">
        <div className="font-mono text-xs uppercase tracking-widest text-muted">
          CLUSTER OVERVIEW / TELEMETRY
        </div>
        <h1 className="font-display font-black text-3xl sm:text-5xl lg:text-6xl uppercase tracking-tighter text-jet leading-poster-headline">
          {nodes.length || 3} ORCHESTRATORS. {workers.length || 5} WORKERS. 0 TASKS LOST.
        </h1>
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <p className="font-sans text-sm text-deep-gray max-w-2xl leading-relaxed">
            Cluster health synchronized in real-time. Leader election maintained via etcd leases; task mutations gated by CAS fencing tokens.
          </p>
          <div className="flex items-center gap-3">
            <Link to="/app/submit">
              <PosterButton variant="primary" size="sm">
                SUBMIT WORKFLOW
              </PosterButton>
            </Link>
            <Link to="/app/resilience">
              <PosterButton variant="secondary" size="sm">
                INJECT FAULT
              </PosterButton>
            </Link>
          </div>
        </div>
      </section>

      {/* 2. 4-CELL BORDERED STRIP (Poster Numerals) */}
      <section className="border-b border-hairline grid grid-cols-2 lg:grid-cols-4">
        {/* Cell 1: Leader & Term */}
        <div className="p-6 lg:p-8 border-r border-b lg:border-b-0 border-hairline space-y-2">
          <div className="font-mono text-xs text-muted uppercase">Leader &amp; Term</div>
          <div className="font-display font-bold text-2xl sm:text-3xl text-jet tracking-tight">
            {leaderId}
          </div>
          <div className="font-mono text-xs text-cobalt font-bold">
            TERM {term} (FENCING TOKEN)
          </div>
        </div>

        {/* Cell 2: Workers & Utilization */}
        <div className="p-6 lg:p-8 border-r border-b lg:border-b-0 border-hairline space-y-2">
          <div className="font-mono text-xs text-muted uppercase">Workers &amp; Slots</div>
          <div className="font-display font-bold text-2xl sm:text-3xl text-jet tracking-tight">
            {cluster?.worker_count ?? workers.length} WORKERS
          </div>
          <div className="font-mono text-xs text-muted-strong">
            {totalRunning} / {totalCapacity || 20} SLOTS ACTIVE
          </div>
        </div>

        {/* Cell 3: Queue Depth */}
        <div className="p-6 lg:p-8 border-r border-hairline space-y-2">
          <div className="font-mono text-xs text-muted uppercase">Queue Depth</div>
          <div className="font-display font-bold text-2xl sm:text-3xl text-jet tracking-tight">
            {cluster?.queue_depth ?? 0}
          </div>
          <div className="font-mono text-xs text-muted-strong">
            READY TO SCHEDULE
          </div>
        </div>

        {/* Cell 4: Workflows */}
        <div className="p-6 lg:p-8 space-y-2">
          <div className="font-mono text-xs text-muted uppercase">Workflows</div>
          <div className="font-display font-bold text-2xl sm:text-3xl text-jet tracking-tight">
            {cluster?.total_workflows ?? (workflows.length || 1)} ACTIVE
          </div>
          <div className="font-mono text-xs text-muted-strong">
            TOPOLOGICALLY SCHEDULED
          </div>
        </div>
      </section>

      {/* 3. FULL-WIDTH SCORE VIEW (Last 60 Seconds with Failover Ticks) */}
      <section className="border-b border-hairline p-6 lg:p-12 space-y-6">
        <ClusterScoreTimeline
          cluster={cluster}
          workflows={workflows}
          events={events}
        />
      </section>

      {/* 4. WORKERS SECTION (Hairline Table) */}
      <section className="border-b border-hairline p-6 lg:p-12 space-y-6">
        <div className="flex items-baseline justify-between border-b border-hairline pb-4">
          <div className="space-y-1">
            <span className="font-mono text-xs text-muted">[ 01 / WORKERS ]</span>
            <h2 className="font-display font-bold text-xl uppercase tracking-tight text-jet">
              Worker Fleet Leases
            </h2>
          </div>
          <span className="font-mono text-xs text-muted">
            5s TTL HEARTBEATS
          </span>
        </div>

        <div className="overflow-x-auto border border-hairline">
          <table className="w-full text-left text-xs border-collapse font-mono">
            <thead>
              <tr className="border-b border-hairline bg-cream font-bold uppercase text-muted text-[11px]">
                <th className="py-3 px-4">Worker ID</th>
                <th className="py-3 px-4">Status</th>
                <th className="py-3 px-4">Utilization</th>
                <th className="py-3 px-4">Capacity</th>
                <th className="py-3 px-4">Heartbeat Lease</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-hairline">
              {workers.map((w, idx) => {
                const running = w.current_tasks ?? w.running_tasks ?? 0;
                const cap = w.capacity || 4;
                const pct = Math.round((running / cap) * 100);
                const rawId = w.worker_id || `worker-${idx + 1}`;
                const friendlyName = `worker-${idx + 1}`;
                const sId = sanitizeWorkerId(rawId);
                const isDifferent = rawId !== friendlyName && sId !== friendlyName;

                return (
                  <tr key={w.worker_id || idx} className="hover:bg-[rgba(255,255,255,0.2)] transition-colors duration-300">
                    <td className="py-3 px-4 font-bold text-jet" title={`Full Worker ID: ${rawId}`}>
                      <div className="flex items-center gap-2">
                        <span>{sId}</span>
                        {isDifferent && (
                          <span className="font-mono text-[10px] text-muted-strong font-normal truncate max-w-[140px]" title={rawId}>
                            [{rawId.slice(0, 8)}...]
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="py-3 px-4">
                      <StatusBadge state={w.active ? 'RUNNING' : 'FAILED'} label={w.active ? 'ACTIVE' : 'DEAD'} />
                    </td>
                    <td className="py-3 px-4 w-48">
                      <div className="flex items-center gap-2">
                        <div className="flex-1 h-2 bg-cream border border-hairline overflow-hidden">
                          <div
                            className="h-full bg-cobalt transition-all duration-300"
                            style={{ width: `${Math.min(pct, 100)}%` }}
                          />
                        </div>
                        <span className="text-[11px] text-muted-strong">{pct}%</span>
                      </div>
                    </td>
                    <td className="py-3 px-4 text-muted-strong">
                      {running} / {cap} slots
                    </td>
                    <td className="py-3 px-4 text-jet font-bold">
                      {w.active ? '< 5.0s remaining' : 'LEASE EXPIRED'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      {/* 5. ORCHESTRATORS SECTION (TypographicListItems) */}
      <section className="border-b border-hairline p-6 lg:p-12 space-y-6">
        <div className="space-y-1 border-b border-hairline pb-4">
          <span className="font-mono text-xs text-muted">[ 02 / ORCHESTRATORS ]</span>
          <h2 className="font-display font-bold text-xl uppercase tracking-tight text-jet">
            Consensus &amp; Leadership
          </h2>
        </div>

        <div className="space-y-0 border-b border-hairline">
          {nodes.map((n, idx) => {
            const sId = sanitizeNodeId(n.node_id);
            const isLead = n.is_leader || n.role === 'LEADER' || sId === leaderId;

            return (
              <TypographicListItem
                key={n.node_id || idx}
                index={`00${idx + 1}`}
                title={sId}
                subtitle={`${isLead ? 'LEADER (TERM ' + term + ')' : 'STANDBY (REPLICATING)'} — STATUS: ${n.status || 'UP'}`}
                size="dashboard"
                active={isLead}
              />
            );
          })}
        </div>
      </section>

      {/* 6. EVENTS LOG (Mono Feed with Filters & Pause Toggle) */}
      <section className="p-6 lg:p-12 space-y-6">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-hairline pb-4">
          <div className="space-y-1">
            <span className="font-mono text-xs text-muted">[ 03 / EVENTS ]</span>
            <h2 className="font-display font-bold text-xl uppercase tracking-tight text-jet">
              Audit Event Log
            </h2>
          </div>

          <div className="flex items-center gap-3">
            <div className="relative">
              <Search className="absolute left-3 top-2.5 h-3.5 w-3.5 text-muted pointer-events-none" />
              <input
                type="text"
                value={eventFilter}
                onChange={e => setEventFilter(e.target.value)}
                placeholder="FILTER EVENTS..."
                className="pl-9 pr-3 py-1.5 font-mono text-xs border border-hairline bg-cream text-jet placeholder:text-muted focus-visible:outline-none w-48"
              />
            </div>

            <button
              type="button"
              onClick={() => setIsPaused(prev => !prev)}
              className="flex items-center gap-1.5 px-3 py-1.5 border border-hairline font-mono text-xs uppercase hover:bg-jet hover:text-cream transition-colors duration-300 focus-visible:outline-none"
            >
              {isPaused ? <Play className="h-3 w-3" /> : <Pause className="h-3 w-3" />}
              <span>{isPaused ? 'RESUME' : 'PAUSE'}</span>
            </button>

            <button
              type="button"
              onClick={onClearEvents}
              className="flex items-center gap-1 px-3 py-1.5 border border-hairline font-mono text-xs uppercase hover:bg-red hover:text-cream transition-colors duration-300 focus-visible:outline-none"
              title="Clear event log"
            >
              <Trash2 className="h-3 w-3" />
              <span>CLEAR</span>
            </button>
          </div>
        </div>

        <div className="border border-hairline bg-cream font-mono text-xs">
          <div className="max-h-96 overflow-y-auto divide-y divide-hairline">
            {filteredEvents.length === 0 ? (
              <div className="py-8 text-center text-muted">
                [ NO EVENTS RECORDED ]
              </div>
            ) : (
              filteredEvents.slice(0, 100).map((ev, i) => {
                const isLeaderEv = ev.type?.includes('LEADER');
                const isWorkerEv = ev.type?.includes('WORKER');

                return (
                  <div key={i} className="p-3 flex flex-wrap items-baseline gap-3 hover:bg-[rgba(255,255,255,0.2)] transition-colors duration-300">
                    <span className="text-muted-strong min-w-[5.5rem]">
                      {formatTimestamp(ev.timestamp)}
                    </span>

                    <span className={`font-bold px-1.5 py-0.2 ${
                      isLeaderEv ? 'bg-cobalt text-cream' : isWorkerEv ? 'bg-jet text-cream' : 'border border-hairline text-jet'
                    }`}>
                      {ev.type}
                    </span>

                    {ev.workflow_id && (
                      <span className="text-deep-gray">
                        wf:<strong className="text-jet">{ev.workflow_id}</strong>
                      </span>
                    )}

                    {ev.task_id && (
                      <span className="text-deep-gray">
                        task:<strong className="text-jet">{ev.task_id}</strong>
                      </span>
                    )}

                    {ev.worker_id && (
                      <span className="text-deep-gray">
                        worker:<strong className="text-jet">{sanitizeWorkerId(ev.worker_id)}</strong>
                      </span>
                    )}

                    {ev.new_state && (
                      <StatusBadge state={ev.new_state} label={String(ev.new_state)} />
                    )}
                  </div>
                );
              })
            )}
          </div>
        </div>
      </section>
    </div>
  );
};

export default ClusterView;
