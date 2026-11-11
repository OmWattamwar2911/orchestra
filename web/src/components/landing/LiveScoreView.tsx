import React, { useState, useEffect, useRef } from 'react';

export const LiveScoreView: React.FC = () => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [isVisible, setIsVisible] = useState(true);
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(false);
  const [tick, setTick] = useState(0); // 0 to 160 (16 seconds in 100ms ticks)

  // Detect prefers-reduced-motion
  useEffect(() => {
    if (typeof window !== 'undefined') {
      const mediaQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
      setPrefersReducedMotion(mediaQuery.matches);
      const listener = (e: MediaQueryListEvent) => setPrefersReducedMotion(e.matches);
      mediaQuery.addEventListener('change', listener);
      return () => mediaQuery.removeEventListener('change', listener);
    }
  }, []);

  // Pause when off-screen using IntersectionObserver
  useEffect(() => {
    if (!containerRef.current) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        setIsVisible(entry.isIntersecting);
      },
      { threshold: 0.1 }
    );
    observer.observe(containerRef.current);
    return () => observer.disconnect();
  }, []);

  // 16-second looping timer (160 ticks at 100ms)
  useEffect(() => {
    if (prefersReducedMotion || !isVisible) return;

    const interval = setInterval(() => {
      setTick(prev => (prev >= 160 ? 0 : prev + 1));
    }, 100);

    return () => clearInterval(interval);
  }, [isVisible, prefersReducedMotion]);

  // If reduced motion, show final state (tick = 160)
  const currentTick = prefersReducedMotion ? 160 : tick;
  const seconds = (currentTick * 0.1).toFixed(1);

  // States at various points in time:
  // T=0 to T=4s: fetch runs and completes on Worker 1.
  // T=4s to T=8s: transformA on W1, transformB on W3.
  // T=8s (tick 80): Worker 3 HALTS!
  // T=8.5s (tick 85): transformB migrates to Worker 1!
  // T=11s (tick 110): transformA and transformB complete.
  // T=11s to T=13.5s: merge on Worker 2.
  // T=13.5s to T=15.5s: publish on Worker 1.
  // T >= 15.5s: Completed.

  const isWorker3Dead = currentTick >= 80;
  const isTransformBMigrated = currentTick >= 85;
  const isTransformBCompleted = currentTick >= 110;
  const isFetchCompleted = currentTick >= 35;
  const isTransformACompleted = currentTick >= 105;
  const isMergeRunning = currentTick >= 110 && currentTick < 135;
  const isMergeCompleted = currentTick >= 135;
  const isPublishRunning = currentTick >= 135 && currentTick < 155;
  const isPublishCompleted = currentTick >= 155;

  return (
    <div ref={containerRef} className="w-full space-y-4">
      {/* Time Header & Progress Tick */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between border-b border-hairline pb-3 font-mono text-xs">
        <div className="flex items-center gap-3">
          <span className="font-bold text-jet">[ WORKFLOW: diamond-pipeline-sample ]</span>
          <span className="text-muted">T = {seconds}s / 16.0s</span>
        </div>
        <div className="flex items-center gap-4 text-muted text-[11px] mt-2 sm:mt-0">
          <span className="flex items-center gap-1.5">
            <span className="w-2.5 h-2.5 bg-cobalt inline-block" /> RUNNING
          </span>
          <span className="flex items-center gap-1.5">
            <span className="w-2.5 h-2.5 bg-jet inline-block" /> COMPLETED
          </span>
          <span className="flex items-center gap-1.5">
            <span className="w-2.5 h-2.5 bg-red inline-block" /> DEAD
          </span>
        </div>
      </div>

      {/* Flat Score View Staves (Lanes) */}
      <div className="border border-hairline bg-cream font-mono text-xs divide-y divide-hairline">
        {/* Lane: Worker 1 */}
        <div className="flex items-center h-14 sm:h-16 relative">
          <div className="w-24 sm:w-32 px-4 py-2 border-r border-hairline font-bold text-jet flex-shrink-0 flex items-center justify-between bg-cream">
            <span>worker-1</span>
            <span className="text-[10px] text-muted">4 slots</span>
          </div>
          <div className="flex-1 h-full relative overflow-hidden flex items-center px-2 sm:px-4 gap-2">
            {/* Task: fetch (0-3.5s) */}
            <div
              className={`h-8 px-2.5 flex items-center gap-1.5 border border-jet text-[11px] font-bold ${
                isFetchCompleted ? 'bg-jet text-cream' : currentTick > 5 ? 'bg-cobalt text-cream' : 'border-hairline text-muted'
              }`}
            >
              <span>{isFetchCompleted ? '■' : '▶'}</span>
              <span>fetch</span>
              <span className="text-[9px] opacity-80">{isFetchCompleted ? 'DONE' : 'RUN'}</span>
            </div>

            {/* Task: transformA (3.5s - 10.5s) */}
            {currentTick >= 35 && (
              <div
                className={`h-8 px-2.5 flex items-center gap-1.5 border border-jet text-[11px] font-bold ${
                  isTransformACompleted ? 'bg-jet text-cream' : 'bg-cobalt text-cream'
                }`}
              >
                <span>{isTransformACompleted ? '■' : '▶'}</span>
                <span>transformA</span>
                <span className="text-[9px] opacity-80">{isTransformACompleted ? 'DONE' : 'RUN'}</span>
              </div>
            )}

            {/* Task: transformB (MIGRATED from Worker 3 at T=8.5s) */}
            {isTransformBMigrated && (
              <div
                className={`h-8 px-2.5 flex items-center gap-1.5 border border-cobalt text-[11px] font-bold animate-pulse ${
                  isTransformBCompleted ? 'bg-jet text-cream border-jet' : 'bg-cobalt text-cream'
                }`}
              >
                <span className="text-amber">⤾</span>
                <span>transformB (migrated)</span>
                <span className="text-[9px] opacity-80">{isTransformBCompleted ? 'DONE' : 'RUN'}</span>
              </div>
            )}

            {/* Task: publish (13.5s - 15.5s) */}
            {currentTick >= 135 && (
              <div
                className={`h-8 px-2.5 flex items-center gap-1.5 border border-jet text-[11px] font-bold ${
                  isPublishCompleted ? 'bg-jet text-cream' : isPublishRunning ? 'bg-cobalt text-cream' : 'border-hairline text-muted'
                }`}
              >
                <span>{isPublishCompleted ? '■' : '▶'}</span>
                <span>publish</span>
                <span className="text-[9px] opacity-80">{isPublishCompleted ? 'DONE' : 'RUN'}</span>
              </div>
            )}
          </div>
        </div>

        {/* Lane: Worker 2 */}
        <div className="flex items-center h-14 sm:h-16 relative">
          <div className="w-24 sm:w-32 px-4 py-2 border-r border-hairline font-bold text-jet flex-shrink-0 flex items-center justify-between bg-cream">
            <span>worker-2</span>
            <span className="text-[10px] text-muted">4 slots</span>
          </div>
          <div className="flex-1 h-full relative overflow-hidden flex items-center px-2 sm:px-4 gap-2">
            {/* Task: merge (11s - 13.5s) */}
            {currentTick >= 110 && (
              <div
                className={`h-8 px-2.5 flex items-center gap-1.5 border border-jet text-[11px] font-bold ${
                  isMergeCompleted ? 'bg-jet text-cream' : isMergeRunning ? 'bg-cobalt text-cream' : 'border-hairline text-muted'
                }`}
              >
                <span>{isMergeCompleted ? '■' : '▶'}</span>
                <span>merge</span>
                <span className="text-[9px] opacity-80">{isMergeCompleted ? 'DONE' : 'RUN'}</span>
              </div>
            )}
            {currentTick < 110 && (
              <span className="text-[11px] text-muted-strong">[ idle - awaiting upstream barrier ]</span>
            )}
          </div>
        </div>

        {/* Lane: Worker 3 (DIES at T=8.0s) */}
        <div className={`flex items-center h-14 sm:h-16 relative transition-colors duration-300 ${
          isWorker3Dead ? 'bg-red/10' : ''
        }`}>
          <div className="w-24 sm:w-32 px-4 py-2 border-r border-hairline font-bold text-jet flex-shrink-0 flex items-center justify-between bg-cream">
            <span className={isWorker3Dead ? 'text-red line-through' : ''}>worker-3</span>
            <span className="text-[10px] text-muted">4 slots</span>
          </div>
          <div className="flex-1 h-full relative overflow-hidden flex items-center px-2 sm:px-4 gap-2">
            {/* transformB running here before death */}
            {currentTick >= 35 && !isTransformBMigrated && (
              <div
                className={`h-8 px-2.5 flex items-center gap-1.5 border text-[11px] font-bold ${
                  isWorker3Dead ? 'bg-red text-cream border-red' : 'bg-cobalt text-cream border-jet'
                }`}
              >
                <span>{isWorker3Dead ? '✕' : '▶'}</span>
                <span>transformB</span>
                <span className="text-[9px] opacity-80">{isWorker3Dead ? 'EVICTED' : 'RUN'}</span>
              </div>
            )}

            {isWorker3Dead && (
              <span className="text-[11px] font-bold text-red">
                [ PROCESS TERMINATED — HEARTBEAT EXPIRED — TASKS VACATED ]
              </span>
            )}

            {!isWorker3Dead && currentTick < 35 && (
              <span className="text-[11px] text-muted-strong">[ ready ]</span>
            )}
          </div>
        </div>

        {/* Lane: Worker 4 */}
        <div className="flex items-center h-14 sm:h-16 relative">
          <div className="w-24 sm:w-32 px-4 py-2 border-r border-hairline font-bold text-jet flex-shrink-0 flex items-center justify-between bg-cream">
            <span>worker-4</span>
            <span className="text-[10px] text-muted">4 slots</span>
          </div>
          <div className="flex-1 h-full relative overflow-hidden flex items-center px-2 sm:px-4">
            <span className="text-[11px] text-muted-strong">[ standby replica ]</span>
          </div>
        </div>

        {/* Lane: Worker 5 */}
        <div className="flex items-center h-14 sm:h-16 relative">
          <div className="w-24 sm:w-32 px-4 py-2 border-r border-hairline font-bold text-jet flex-shrink-0 flex items-center justify-between bg-cream">
            <span>worker-5</span>
            <span className="text-[10px] text-muted">4 slots</span>
          </div>
          <div className="flex-1 h-full relative overflow-hidden flex items-center px-2 sm:px-4">
            <span className="text-[11px] text-muted-strong">[ standby replica ]</span>
          </div>
        </div>
      </div>

      {/* One-line Plain Language Caption (from Spec) */}
      <div className="border border-hairline p-4 bg-cream flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <p className="font-mono text-xs text-jet leading-relaxed">
          <strong className="text-cobalt font-bold">T={seconds}s: </strong>
          {currentTick < 80 ? (
            <span>Workers 1 and 3 are executing branch transforms in parallel.</span>
          ) : currentTick < 110 ? (
            <span className="font-bold text-red">
              Worker 3 stopped. Its tasks moved to worker 1. Nothing was lost or run twice.
            </span>
          ) : currentTick < 155 ? (
            <span>Worker 1 finished transformB; downstream merge and publish dispatched without disruption.</span>
          ) : (
            <span className="font-bold text-jet">
              Workflow complete. 5 of 5 tasks safely committed to terminal state.
            </span>
          )}
        </p>

        <span className="font-mono text-[10px] text-muted uppercase">
          {prefersReducedMotion ? '[ REDUCED MOTION STATIC FRAME ]' : '[ LOOPING SIMULATION ]'}
        </span>
      </div>
    </div>
  );
};

export default LiveScoreView;
