import React from 'react';
import { X } from 'lucide-react';

export interface StatusBadgeProps {
  state?: string | number;
  label?: string;
  className?: string;
}

export const StatusBadge: React.FC<StatusBadgeProps> = ({
  state,
  label,
  className = '',
}) => {
  // Normalize state
  const stateStr = String(state || '').toUpperCase();
  const isRunning = stateStr === '4' || stateStr === 'RUNNING';
  const isCompleted = stateStr === '5' || stateStr === 'COMPLETED' || stateStr === 'SUCCESS';
  const isFailed = stateStr === '6' || stateStr === 'FAILED';
  const isRetrying = stateStr === '3' || stateStr === 'RETRYING' || stateStr === 'RETRY';
  const isCancelled = stateStr === '7' || stateStr === 'CANCELLED' || stateStr === 'SKIPPED';

  if (isRunning) {
    return (
      <span className={`inline-flex items-center gap-1 bg-cobalt text-cream px-2 py-0.5 font-mono text-[11px] font-bold uppercase tracking-wider ${className}`}>
        <span>■</span>
        <span>{label || 'RUN'}</span>
      </span>
    );
  }

  if (isCompleted) {
    return (
      <span className={`inline-flex items-center gap-1 bg-jet text-cream px-2 py-0.5 font-mono text-[11px] font-bold uppercase tracking-wider ${className}`}>
        <span>■</span>
        <span>{label || 'DONE'}</span>
      </span>
    );
  }

  if (isRetrying) {
    return (
      <span className={`inline-flex items-center gap-1 hatch-amber border border-amber text-amber px-2 py-0.5 font-mono text-[11px] font-bold uppercase tracking-wider ${className}`}>
        <span>///</span>
        <span>{label || 'RETRY'}</span>
      </span>
    );
  }

  if (isFailed) {
    return (
      <span className={`inline-flex items-center gap-1 bg-red text-cream px-2 py-0.5 font-mono text-[11px] font-bold uppercase tracking-wider ${className}`}>
        <X className="h-3 w-3 stroke-[3]" />
        <span>{label || 'FAIL'}</span>
      </span>
    );
  }

  if (isCancelled) {
    return (
      <span className={`inline-flex items-center gap-1 hatch-cancelled border border-hairline text-muted px-2 py-0.5 font-mono text-[11px] font-bold uppercase tracking-wider line-through ${className}`}>
        <span>{label || 'CANCEL'}</span>
      </span>
    );
  }

  // Queued / Pending / Ready
  return (
    <span className={`inline-flex items-center gap-1 border border-hairline bg-transparent text-muted-strong px-2 py-0.5 font-mono text-[11px] font-bold uppercase tracking-wider ${className}`}>
      <span>□</span>
      <span>{label || 'WAIT'}</span>
    </span>
  );
};

export default StatusBadge;
