import React, { useState, useEffect } from 'react';
import { ClusterStatus, WorkflowWithTasks, EventRecord } from '../../types';

interface ClusterScoreTimelineProps {
  cluster: ClusterStatus | null;
  workflows: WorkflowWithTasks[];
  events: EventRecord[];
}

export const ClusterScoreTimeline: React.FC<ClusterScoreTimelineProps> = ({
  cluster,
  workflows,
  events,
}) => {
  const [now, setNow] = useState(Date.now());

  // Slide timeline every 1 second
  useEffect(() => {
    const interval = setInterval(() => {
      setNow(Date.now());
    }, 1000);
    return () => clearInterval(interval);
  }, []);

  const windowMs = 60000; // 60 seconds
  const startMs = now - windowMs;

  const workers = cluster?.workers || [
    { worker_id: 'worker-1', capacity: 4, current_tasks: 1, running_tasks: 1, active: true },
    { worker_id: 'worker-2', capacity: 4, current_tasks: 1, running_tasks: 1, active: true },
    { worker_id: 'worker-3', capacity: 4, current_tasks: 0, running_tasks: 0, active: true },
    { worker_id: 'worker-4', capacity: 4, current_tasks: 0, running_tasks: 0, active: true },
    { worker_id: 'worker-5', capacity: 4, current_tasks: 0, running_tasks: 0, active: true },
  ];

  // Filter events within last 60s
  const recentMarkers = events.filter(e => {
    if (!e.timestamp) return false;
    const t = e.timestamp > 10000000000 ? e.timestamp : e.timestamp * 1000;
    return t >= startMs && t <= now && (
      e.type?.includes('LEADER') || 
      e.type?.includes('WORKER_KILLED') ||
      e.type?.includes('MIGRATED')
    );
  });

  // Calculate percentage along the 60-second X-axis
  const getXPercent = (timestamp: number) => {
    const t = timestamp > 10000000000 ? timestamp : timestamp * 1000;
    const offset = t - startMs;
    const clamped = Math.max(0, Math.min(offset, windowMs));
    return (clamped / windowMs) * 100;
  };

  return (
    <div className="w-full space-y-4">
      {/* Timeline Header & Scale */}
      <div className="flex items-baseline justify-between border-b border-hairline pb-2 font-mono text-xs">
        <div className="flex items-center gap-3">
          <span className="font-bold text-jet">[ SCORE VIEW / LAST 60 SECONDS ]</span>
          <span className="text-muted">SLIDING EXECUTION WINDOW</span>
        </div>
        <div className="flex items-center gap-4 text-muted text-[11px]">
          <span className="flex items-center gap-1">
            <span className="w-2.5 h-2.5 bg-cobalt inline-block" /> RUNNING
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2.5 h-2.5 bg-jet inline-block" /> COMPLETED
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 border border-cobalt inline-block" /> FAILOVER TICK
          </span>
        </div>
      </div>

      {/* Main Score Staves (Lanes) */}
      <div className="border border-hairline bg-cream font-mono text-xs relative divide-y divide-hairline">
        {/* Event Markers Overlay (Vertical Hairlines across staves) */}
        {recentMarkers.map((ev, i) => {
          const xPct = getXPercent(ev.timestamp || now);
          const isLeader = ev.type?.includes('LEADER');
          const isWorkerDeath = ev.type?.includes('WORKER_KILLED');

          return (
            <div
              key={i}
              className="absolute top-0 bottom-0 z-20 pointer-events-none flex flex-col justify-between"
              style={{ left: `${xPct}%` }}
            >
              <div
                className={`text-[9px] font-bold px-1 py-0.2 whitespace-nowrap -translate-x-1/2 ${
                  isLeader ? 'bg-cobalt text-cream' : isWorkerDeath ? 'bg-red text-cream' : 'bg-jet text-cream'
                }`}
              >
                {isLeader ? `TERM ${ev.term || 2}` : isWorkerDeath ? `${ev.worker_id} DEAD` : 'MIGRATED'}
              </div>
              <div
                className={`w-[1px] h-full ${
                  isLeader ? 'bg-cobalt' : isWorkerDeath ? 'bg-red' : 'bg-jet'
                }`}
              />
            </div>
          );
        })}

        {/* Worker Lanes */}
        {workers.map((w, idx) => {
          const wId = w.worker_id.replace(/^distributed-scheduler-/, '').replace(/[-_]/g, '-').toLowerCase();

          // Find tasks that ran or are running on this worker in the last 60s
          const workerTasks: Array<{
            id: string;
            state: number;
            start: number;
            end: number;
            type: string;
          }> = [];

          workflows.forEach(wf => {
            wf.tasks.forEach(t => {
              const assigned = (t.status.assigned_worker || '').replace(/^distributed-scheduler-/, '').replace(/[-_]/g, '-').toLowerCase();
              if (assigned === wId) {
                const s = t.status.started_at ? (t.status.started_at > 10000000000 ? t.status.started_at : t.status.started_at * 1000) : now - 15000;
                const e = t.status.completed_at ? (t.status.completed_at > 10000000000 ? t.status.completed_at : t.status.completed_at * 1000) : now;
                if (e >= startMs) {
                  workerTasks.push({
                    id: t.spec.id,
                    state: t.status.state,
                    start: Math.max(s, startMs),
                    end: Math.min(e, now),
                    type: t.spec.type || 'task',
                  });
                }
              }
            });
          });

          return (
            <div key={w.worker_id || idx} className="flex items-center h-12 sm:h-14 relative">
              {/* Lane Header */}
              <div className="w-24 sm:w-28 px-3 py-1.5 border-r border-hairline font-bold text-jet flex-shrink-0 flex items-center justify-between bg-cream z-10">
                <span className={w.active ? 'text-jet' : 'text-red line-through'}>{wId}</span>
                <span className="text-[10px] text-muted">{w.capacity || 4}s</span>
              </div>

              {/* Lane Content Track */}
              <div className="flex-1 h-full relative overflow-hidden">
                {/* 15s grid division guides */}
                <div className="absolute inset-0 grid grid-cols-4 pointer-events-none divide-x divide-hairline/40" />

                {workerTasks.map((t, tIdx) => {
                  const leftPct = getXPercent(t.start);
                  const rightPct = getXPercent(t.end);
                  const widthPct = Math.max(rightPct - leftPct, 4);
                  const isCompleted = t.state === 5;

                  return (
                    <div
                      key={tIdx}
                      style={{
                        left: `${leftPct}%`,
                        width: `${widthPct}%`,
                      }}
                      className={`absolute top-2 bottom-2 border px-2 flex items-center gap-1.5 text-[10px] font-mono font-bold truncate z-10 ${
                        isCompleted
                          ? 'bg-jet text-cream border-jet'
                          : 'bg-cobalt text-cream border-cobalt'
                      }`}
                      title={`${t.id} (${isCompleted ? 'COMPLETED' : 'RUNNING'})`}
                    >
                      <span>{isCompleted ? '■' : '▶'}</span>
                      <span className="truncate">{t.id}</span>
                    </div>
                  );
                })}

                {workerTasks.length === 0 && (
                  <div className="h-full flex items-center px-4 text-[10px] text-muted">
                    {w.active ? '[ idle ]' : '[ PROCESS TERMINATED — LEASE VACATED ]'}
                  </div>
                )}
              </div>
            </div>
          );
        })}

        {/* Timeline Bottom Axis with Ticks */}
        <div className="flex items-center h-8 bg-cream border-t border-hairline">
          <div className="w-24 sm:w-28 px-3 border-r border-hairline font-mono text-[10px] text-muted">
            TIME
          </div>
          <div className="flex-1 grid grid-cols-4 font-mono text-[10px] text-muted px-2">
            <span className="text-left">-60s</span>
            <span className="text-left">-45s</span>
            <span className="text-left">-30s</span>
            <div className="flex justify-between">
              <span>-15s</span>
              <span className="text-jet font-bold">NOW</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ClusterScoreTimeline;
