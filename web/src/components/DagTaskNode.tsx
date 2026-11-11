import React, { memo } from 'react';
import { Handle, Position, NodeProps } from 'reactflow';
import { Task, TaskState } from '../types';
import { X } from 'lucide-react';

export interface DagNodeData {
  task: Task;
  isSelected?: boolean;
}

export const DagTaskNode: React.FC<NodeProps<DagNodeData>> = memo(({ data, selected }) => {
  const task = data.task;
  const spec = task?.spec || { id: 'unknown', dependencies: [] };
  const status = task?.status || { state: 1, current_attempt: 0 };
  const stateNum = status.state ?? 1;

  const isRunning = stateNum === TaskState.RUNNING || stateNum === TaskState.ASSIGNED;
  const isCompleted = stateNum === TaskState.COMPLETED;
  const isFailed = stateNum === TaskState.FAILED || stateNum === TaskState.DEAD_LETTER;
  const isRetrying = stateNum === 3;
  const isCancelled = stateNum === TaskState.SKIPPED || stateNum === TaskState.CANCELLED;

  let bgClass = 'bg-cream border-hairline text-jet';
  let badgeLabel = 'WAIT';
  let badgeClass = 'border border-hairline text-muted-strong';

  if (isRunning) {
    bgClass = 'bg-cobalt text-cream border-cobalt';
    badgeLabel = 'RUN';
    badgeClass = 'bg-jet text-cream';
  } else if (isCompleted) {
    bgClass = 'bg-jet text-cream border-jet';
    badgeLabel = 'DONE';
    badgeClass = 'bg-cream text-jet';
  } else if (isFailed) {
    bgClass = 'bg-red text-cream border-red';
    badgeLabel = 'FAIL';
    badgeClass = 'bg-cream text-red';
  } else if (isRetrying) {
    bgClass = 'hatch-amber border-amber text-jet';
    badgeLabel = 'RETRY';
    badgeClass = 'bg-amber text-cream';
  } else if (isCancelled) {
    bgClass = 'hatch-cancelled border-hairline text-muted';
    badgeLabel = 'CANCEL';
    badgeClass = 'border border-hairline text-muted';
  }

  const workerClean = (status.assigned_worker || '')
    .replace(/^distributed-scheduler-/, '')
    .replace(/[-_]/g, '-')
    .toLowerCase();

  return (
    <div
      className={`w-56 p-3.5 border transition-all duration-300 font-mono text-xs ${bgClass} ${
        selected ? 'ring-2 ring-cobalt ring-offset-2 ring-offset-cream' : ''
      }`}
    >
      <Handle
        type="target"
        position={Position.Top}
        className="!w-2 !h-2 !bg-jet !rounded-none !border-hairline"
      />

      <div className="flex items-center justify-between gap-2 border-b border-hairline/40 pb-2 mb-2">
        <span className="font-bold text-sm truncate uppercase tracking-tight">
          {spec.id}
        </span>
        <span className={`px-1.5 py-0.2 text-[10px] font-bold uppercase tracking-wider ${badgeClass} flex items-center gap-1`}>
          {isFailed && <X className="h-3 w-3" />}
          <span>{badgeLabel}</span>
        </span>
      </div>

      <div className="space-y-1 text-[11px] opacity-90 font-sans">
        <div className="flex justify-between">
          <span className="font-mono text-[10px] text-muted">WORKER:</span>
          <span className="font-mono font-bold">{workerClean || 'none'}</span>
        </div>
        <div className="flex justify-between">
          <span className="font-mono text-[10px] text-muted">ATTEMPT:</span>
          <span className="font-mono">{status.current_attempt || 0}/{spec.max_retries || 2}</span>
        </div>
      </div>

      <Handle
        type="source"
        position={Position.Bottom}
        className="!w-2 !h-2 !bg-jet !rounded-none !border-hairline"
      />
    </div>
  );
});

export default DagTaskNode;
