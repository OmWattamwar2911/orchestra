import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { Copy, Check } from 'lucide-react';
import { StickyNav } from '../components/layout/StickyNav';
import { GridSidebarLabel } from '../components/ui/GridSidebarLabel';
import { PosterButton } from '../components/ui/PosterButton';
import { FeatureCell } from '../components/ui/FeatureCell';
import { TypographicListItem } from '../components/ui/TypographicListItem';
import { LiveScoreView } from '../components/landing/LiveScoreView';
import { ArchitectureDiagram } from '../components/landing/ArchitectureDiagram';
import { PRODUCT_NAME } from '../config';

export const LandingPage: React.FC = () => {
  const [copied, setCopied] = useState(false);

  const dockerCommand = `git clone https://github.com/orchestra/orchestra\ncd orchestra\ndocker compose up -d`;

  const handleCopy = () => {
    navigator.clipboard.writeText(dockerCommand);
    setCopied(true);
    setTimeout(() => setCopied(false), 2500);
  };

  return (
    <div className="min-h-screen bg-cream text-jet flex flex-col font-sans selection:bg-cobalt selection:text-cream">
      {/* 1. STICKY NAV (80px height, 1px bottom border, cream at 95% opacity) */}
      <StickyNav mode="landing" />

      {/* 2. HERO / MANIFESTO (min-height 85vh) */}
      <section className="w-full max-w-[1600px] mx-auto border-b border-hairline min-h-[85vh] grid grid-cols-12">
        {/* Cols 1-3: Label sidebar + 16px black square */}
        <div className="col-span-12 lg:col-span-3 border-b lg:border-b-0 lg:border-r border-hairline p-6 lg:p-12">
          <GridSidebarLabel index="01" label="MANIFESTO" square={true} />
        </div>

        {/* Cols 4-12: Massive headline with one key word in cobalt */}
        <div className="col-span-12 lg:col-span-9 p-6 sm:p-12 lg:p-16 flex flex-col justify-between">
          <div className="space-y-6 pt-4 lg:pt-8">
            <h1 className="font-display font-black text-5xl sm:text-7xl lg:text-8xl xl:text-9xl leading-poster-hero tracking-tightest uppercase text-jet">
              RUN TASK GRAPHS.<br />
              <span className="text-cobalt">SURVIVE</span> CRASHES.
            </h1>
          </div>

          {/* 2-Column bottom grid: 400px paragraph + CTA cluster */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-8 pt-12 lg:pt-20 border-t border-hairline mt-12">
            <p className="font-sans text-lg text-deep-gray leading-relaxed max-w-[400px]">
              A fault-tolerant distributed DAG workflow scheduler engine with Raft leader election, monotonic CAS fencing tokens, and leased worker execution. Built to eliminate single points of failure.
            </p>

            <div className="flex flex-col sm:flex-row items-start sm:items-center gap-6">
              <Link to="/app">
                <PosterButton variant="primary">
                  OPEN THE LIVE DASHBOARD
                </PosterButton>
              </Link>
              <a
                href="https://github.com"
                target="_blank"
                rel="noreferrer"
              >
                <PosterButton variant="link">
                  SEE THE CODE ON GITHUB
                </PosterButton>
              </a>
            </div>
          </div>
        </div>
      </section>

      {/* 3. LIVE SECTION (Signature View: workers as lanes, tasks as blocks) */}
      <section className="w-full max-w-[1600px] mx-auto border-b border-hairline grid grid-cols-12">
        <div className="col-span-12 lg:col-span-3 border-b lg:border-b-0 lg:border-r border-hairline p-6 lg:p-12">
          <GridSidebarLabel index="02" label="LIVE" />
        </div>

        <div className="col-span-12 lg:col-span-9 p-6 sm:p-12 lg:p-16 space-y-8">
          <div className="space-y-2">
            <span className="font-mono text-xs text-muted">[ REAL-TIME FAULT MIGRATION TIMELINE ]</span>
            <h2 className="font-display font-bold text-3xl sm:text-5xl uppercase tracking-tighter text-jet">
              The Execution Score
            </h2>
            <p className="text-deep-gray text-base max-w-xl font-sans">
              Watch workers execute independent DAG branches in parallel. When a worker halts, in-flight tasks re-queue and migrate instantly without duplicate commits.
            </p>
          </div>

          {/* Dedicated looping Score View */}
          <LiveScoreView />
        </div>
      </section>

      {/* 4. SYSTEM SECTION (Submit. Schedule. Recover.) */}
      <section className="w-full max-w-[1600px] mx-auto border-b border-hairline grid grid-cols-12">
        <div className="col-span-12 lg:col-span-3 border-b lg:border-b-0 lg:border-r border-hairline p-6 lg:p-12">
          <GridSidebarLabel index="03" label="SYSTEM" />
        </div>

        <div className="col-span-12 lg:col-span-9 p-6 sm:p-12 lg:p-16 space-y-12">
          <div className="space-y-2">
            <h2 className="font-display font-bold text-4xl sm:text-6xl leading-poster-headline uppercase text-jet tracking-tighter">
              SUBMIT.<br />
              SCHEDULE.<br />
              RECOVER.
            </h2>
            <p className="text-deep-gray text-base max-w-xl font-sans">
              Three strict execution phases ensuring deterministic workflow completion despite arbitrary process failures.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-0 border border-hairline">
            <FeatureCell
              index="01"
              title="Define a workflow as a graph"
              description="Author task dependencies in declarative YAML or JSON. Orchestra computes strict topological execution levels and validates acyclicity prior to dispatch."
            />
            <FeatureCell
              index="02"
              title="Run it across workers in order"
              description="Workers maintain 5-second renew leases in etcd. Compare-and-swap state checks enforce parent barriers before downstream tasks are triggered."
            />
            <FeatureCell
              index="03"
              title="Recover without lost work"
              description="If a worker process halts, its unfinished tasks return to the ready queue and reschedule on surviving workers. Nothing is lost or executed twice."
            />
          </div>
        </div>
      </section>

      {/* 5. WHY DIFFERENT SECTION (Four TypographicListItems) */}
      <section className="w-full max-w-[1600px] mx-auto border-b border-hairline grid grid-cols-12">
        <div className="col-span-12 lg:col-span-3 border-b lg:border-b-0 lg:border-r border-hairline p-6 lg:p-12">
          <GridSidebarLabel index="04" label="GUARANTEES" />
        </div>

        <div className="col-span-12 lg:col-span-9 p-6 sm:p-12 lg:p-16 space-y-10">
          <div>
            <h2 className="font-display font-bold text-4xl sm:text-6xl leading-poster-headline uppercase text-jet tracking-tighter">
              THIS ISN'T ANOTHER CRON.<br />
              <span className="text-cobalt">FORMAL INVARIANTS.</span>
            </h2>
          </div>

          <div className="border-b border-hairline">
            <TypographicListItem
              index="001"
              title="No lost tasks"
              explanation="Workers hold 5-second heartbeat leases in etcd. If a worker process halts or gets severed from the network, its lease expires, and in-flight tasks immediately return to the ready queue to resume on surviving nodes."
            />
            <TypographicListItem
              index="002"
              title="No out-of-order runs"
              explanation="Topological dependency barriers are computed prior to dispatch and enforced by etcd compare-and-swap operations. No downstream task can ever be dispatched until all parent tasks have committed their final output."
            />
            <TypographicListItem
              index="003"
              title="No zombie leaders"
              explanation="Every newly elected leader receives a strictly increasing monotonic fencing token. Stale partitioned leaders attempting state writes are rejected immediately by CAS version checks."
            />
            <TypographicListItem
              index="004"
              title="No duplicate results"
              explanation="Task execution attempts carry deterministic attempt IDs with compare-and-swap status transition validation. Late-arriving duplicate completions from revived workers are discarded safely."
            />
          </div>
        </div>
      </section>

      {/* 6. PROOF SECTION (Measured Chaos Results from results/run-4001/checker_report.json) */}
      <section className="w-full max-w-[1600px] mx-auto border-b border-hairline grid grid-cols-12">
        <div className="col-span-12 lg:col-span-3 border-b lg:border-b-0 lg:border-r border-hairline p-6 lg:p-12">
          <GridSidebarLabel index="05" label="PROOF" />
        </div>

        <div className="col-span-12 lg:col-span-9 p-6 sm:p-12 lg:p-16 space-y-8">
          <div>
            <span className="font-mono text-xs text-muted">[ RESULTS / RUN-4001 / CHECKER_REPORT.JSON ]</span>
            <h2 className="font-display font-bold text-3xl sm:text-5xl uppercase tracking-tighter text-jet mt-2">
              MEASURED UNDER CHAOS
            </h2>
          </div>

          <div className="grid grid-cols-2 lg:grid-cols-4 gap-0 border border-hairline text-center">
            <div className="p-6 sm:p-8 border-r border-b lg:border-b-0 border-hairline">
              <div className="font-display font-bold text-4xl sm:text-6xl text-jet">20</div>
              <div className="font-mono text-xs text-muted uppercase mt-2">Workflows Completed</div>
            </div>
            <div className="p-6 sm:p-8 border-r border-b lg:border-b-0 border-hairline">
              <div className="font-display font-bold text-4xl sm:text-6xl text-jet">491</div>
              <div className="font-mono text-xs text-muted uppercase mt-2">Tasks Executed</div>
            </div>
            <div className="p-6 sm:p-8 border-r border-hairline">
              <div className="font-display font-bold text-4xl sm:text-6xl text-cobalt">0</div>
              <div className="font-mono text-xs text-muted uppercase mt-2">Order Violations</div>
            </div>
            <div className="p-6 sm:p-8">
              <div className="font-display font-bold text-4xl sm:text-6xl text-cobalt">0</div>
              <div className="font-mono text-xs text-muted uppercase mt-2">Lost Tasks</div>
            </div>
          </div>

          <div className="border border-hairline p-6 font-mono text-xs space-y-2 bg-cream">
            <div className="flex flex-col sm:flex-row justify-between text-muted-strong">
              <span>LEADER FAILOVERS TRIGGERED: <strong className="text-jet">4</strong></span>
              <span>MEAN FAILOVER TIME: <strong className="text-jet">5,308 ms</strong></span>
              <span>DUPLICATES DROPPED: <strong className="text-jet">14</strong></span>
            </div>
            <div className="border-t border-hairline pt-2 text-muted">
              Hardware: 3 Orchestrator nodes, 5 Workers, 3-node etcd cluster under Docker container fault injection (KILL_WORKER, PAUSE_LEADER_ZOMBIE).
            </div>
          </div>
        </div>
      </section>

      {/* 7. HOW IT WORKS / ARCHITECTURE (Line-drawing diagram) */}
      <section className="w-full max-w-[1600px] mx-auto border-b border-hairline grid grid-cols-12">
        <div className="col-span-12 lg:col-span-3 border-b lg:border-b-0 lg:border-r border-hairline p-6 lg:p-12">
          <GridSidebarLabel index="06" label="ARCHITECTURE" />
        </div>

        <div className="col-span-12 lg:col-span-9 p-6 sm:p-12 lg:p-16 space-y-8">
          <div className="space-y-2">
            <span className="font-mono text-xs text-muted">[ FORMAL SPECIFICATION ]</span>
            <h2 className="font-display font-bold text-3xl sm:text-5xl uppercase tracking-tighter text-jet">
              HOW IT WORKS
            </h2>
            <p className="text-deep-gray text-base max-w-xl font-sans">
              Distributed consensus, fencing tokens, and worker lease renewal topology.
            </p>
          </div>

          {/* SVG Line-drawing Diagram */}
          <ArchitectureDiagram />
        </div>
      </section>

      {/* 8. ACCESS SECTION (8xl Headline, Docker commands, black PosterButton) */}
      <section className="w-full max-w-[1600px] mx-auto border-b border-hairline min-h-[50vh] grid grid-cols-12">
        <div className="col-span-12 lg:col-span-3 border-b lg:border-b-0 lg:border-r border-hairline p-6 lg:p-12">
          <GridSidebarLabel index="07" label="ACCESS" />
        </div>

        <div className="col-span-12 lg:col-span-9 p-6 sm:p-12 lg:p-16 flex flex-col justify-between">
          <div className="space-y-6">
            <h2 className="font-display font-black text-5xl sm:text-7xl lg:text-8xl uppercase tracking-tightest leading-poster-headline text-jet">
              START EXPLORING
            </h2>
            <div className="space-y-1 font-sans text-base sm:text-lg text-deep-gray">
              <p>Open the live demo in your browser.</p>
              <p>Run the full cluster locally with Docker.</p>
            </div>

            {/* Flat Copyable Code Block */}
            <div className="border border-hairline bg-cream p-4 sm:p-6 max-w-2xl flex items-center justify-between font-mono text-xs text-jet mt-6">
              <pre className="overflow-x-auto whitespace-pre-wrap">{dockerCommand}</pre>
              <button
                type="button"
                onClick={handleCopy}
                className="ml-4 p-2 border border-hairline hover:bg-jet hover:text-cream transition-colors duration-300 focus-visible:outline-none flex-shrink-0"
                title="Copy docker compose command"
              >
                {copied ? <Check className="h-4 w-4 text-cobalt" /> : <Copy className="h-4 w-4" />}
              </button>
            </div>
          </div>

          <div className="pt-12 flex justify-end">
            <Link to="/app">
              <PosterButton variant="secondary" size="lg">
                OPEN THE LIVE DASHBOARD
              </PosterButton>
            </Link>
          </div>
        </div>
      </section>

      {/* 9. FOOTER (Blue 16-24px square, logo, mono tagline, links) */}
      <footer className="w-full max-w-[1600px] mx-auto px-6 lg:px-12 py-12 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-6">
        <div className="flex items-center gap-3">
          <div className="w-5 h-5 bg-cobalt" aria-hidden="true" />
          <span className="font-display font-bold text-xl uppercase tracking-tight text-jet">
            {PRODUCT_NAME}
          </span>
          <span className="font-mono text-xs text-muted">/ Built to keep running.</span>
        </div>

        <div className="flex flex-wrap items-center gap-6 sm:gap-8 font-mono text-xs uppercase text-muted">
          <span>BUILD: <strong className="text-jet">{typeof __COMMIT_HASH__ !== 'undefined' ? __COMMIT_HASH__ : 'dev'}</strong></span>
          <a
            href="https://github.com"
            target="_blank"
            rel="noreferrer"
            className="hover:text-jet transition-colors duration-300 focus-visible:outline-none"
          >
            GITHUB
          </a>
          <Link
            to="/app/docs"
            className="hover:text-jet transition-colors duration-300 focus-visible:outline-none"
          >
            DOCS
          </Link>
          <span className="text-muted-strong">MIT LICENSE</span>
        </div>
      </footer>
    </div>
  );
};

export default LandingPage;
