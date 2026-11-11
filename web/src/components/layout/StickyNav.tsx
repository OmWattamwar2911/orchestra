import React from 'react';
import { Link } from 'react-router-dom';
import { PRODUCT_NAME } from '../../config';
import { useCluster } from '../../context/ClusterContext';

export interface StickyNavProps {
  mode?: 'landing' | 'app';
}

export const StickyNav: React.FC<StickyNavProps> = ({ mode = 'app' }) => {
  const { cluster, sseStatus, isSimulator } = useCluster();

  const isLeader = cluster?.is_leader ?? false;
  const rawLeader = cluster?.leader?.node_id || (isLeader ? cluster?.node_id : '');
  const displayLeader = rawLeader ? rawLeader.replace(/[-_]/g, '-') : 'orchestrator-1';
  const term = cluster?.term || cluster?.fencing_token || 1;
  const commitHash = typeof __COMMIT_HASH__ !== 'undefined' ? __COMMIT_HASH__ : 'dev';
  const buildTime = typeof __BUILD_TIME__ !== 'undefined' ? __BUILD_TIME__ : '';

  return (
    <header className="sticky top-0 z-50 h-20 w-full border-b border-hairline bg-cream/95 backdrop-blur-md">
      <div className="h-full w-full grid grid-cols-12 items-center px-6 lg:px-12">
        {/* Cols 1-3: Logo */}
        <div className="col-span-4 lg:col-span-3 flex items-center gap-3">
          <Link
            to="/"
            className="flex items-center gap-3 group focus-visible:outline-none"
            title={`Orchestra Poster Modernist UI | Build: ${commitHash} (${buildTime})`}
          >
            <div className="w-4 h-4 bg-jet group-hover:bg-cobalt transition-colors duration-300" aria-hidden="true" />
            <span className="font-display font-black text-xl tracking-tight text-jet uppercase">
              {PRODUCT_NAME}
            </span>
            <span className="hidden sm:inline-block font-mono text-[9px] uppercase px-1.5 py-0.5 border border-hairline text-muted-strong" title={`Build commit: ${commitHash}`}>
              {commitHash}
            </span>
          </Link>
        </div>

        {/* Middle cols: Status text */}
        <div className="hidden md:flex col-span-4 lg:col-span-6 items-center justify-center font-mono text-xs uppercase tracking-widest text-muted">
          {mode === 'landing' ? (
            <div className="flex items-center gap-3">
              <span>SCHEDULER V1.0</span>
              {isSimulator && (
                <span className="text-cobalt font-bold">[ DEMO MODE ]</span>
              )}
            </div>
          ) : (
            <div className="flex items-center gap-4 text-muted">
              <span>
                LEADER <strong className="text-jet font-bold">{displayLeader}</strong> / TERM <strong className="text-jet font-bold">{term}</strong>
              </span>
              <span>/</span>
              <div className="flex items-center gap-1.5">
                <span
                  className={`w-2 h-2 ${
                    sseStatus === 'connected'
                      ? 'bg-cobalt'
                      : sseStatus === 'reconnecting'
                      ? 'bg-amber animate-pulse'
                      : 'bg-red'
                  }`}
                  aria-hidden="true"
                />
                <span className="text-jet font-bold">
                  {isSimulator ? 'DEMO MODE' : sseStatus === 'connected' ? 'LIVE' : sseStatus === 'reconnecting' ? 'RECONNECTING' : 'OFFLINE'}
                </span>
              </div>
            </div>
          )}
        </div>

        {/* Right cols: Navigation links */}
        <div className="col-span-8 lg:col-span-3 flex items-center justify-end gap-6 font-mono text-xs font-bold uppercase tracking-wider text-jet">
          {mode === 'landing' ? (
            <>
              <Link to="/app/docs" className="hover:text-cobalt transition-colors duration-300 focus-visible:outline-none">
                DOCS
              </Link>
              <Link to="/app" className="hover:text-cobalt transition-colors duration-300 focus-visible:outline-none">
                DASHBOARD
              </Link>
              <a
                href="https://github.com"
                target="_blank"
                rel="noreferrer"
                className="hover:text-cobalt transition-colors duration-300 focus-visible:outline-none"
              >
                GITHUB
              </a>
            </>
          ) : (
            <>
              <Link to="/" className="hover:text-cobalt transition-colors duration-300 focus-visible:outline-none">
                MANIFESTO
              </Link>
              <Link to="/app/docs" className="hover:text-cobalt transition-colors duration-300 focus-visible:outline-none">
                DOCS
              </Link>
              <a
                href="https://github.com"
                target="_blank"
                rel="noreferrer"
                className="hidden sm:inline-block hover:text-cobalt transition-colors duration-300 focus-visible:outline-none"
              >
                GITHUB
              </a>
            </>
          )}
        </div>
      </div>
    </header>
  );
};

export default StickyNav;
