import React from 'react';
import { NavLink } from 'react-router-dom';
import { 
  Activity, 
  Server, 
  ShieldAlert, 
  Wifi, 
  WifiOff, 
  RefreshCw,
  LayoutDashboard,
  GitBranch,
  FilePlus2
} from 'lucide-react';
import { ClusterStatus } from '../types';

export type SSEStatus = 'connected' | 'reconnecting' | 'disconnected';

export const isChaosFlagEnabled = (): boolean => {
  if (import.meta.env.DEV) return true;
  if (import.meta.env.VITE_ENABLE_CHAOS === 'true' || import.meta.env.VITE_ENABLE_CHAOS === '1') return true;
  if (typeof window !== 'undefined') {
    const params = new URLSearchParams(window.location.search);
    if (params.get('chaos') === 'true' || params.get('chaos') === '1' || window.location.hash.includes('chaos')) {
      return true;
    }
    if (localStorage.getItem('orchestra_chaos') === 'true') {
      return true;
    }
  }
  return false;
};

interface HeaderProps {
  cluster: ClusterStatus | null;
  sseStatus: SSEStatus;
  chaosOpen: boolean;
  setChaosOpen: (open: boolean | ((prev: boolean) => boolean)) => void;
}

export const Header: React.FC<HeaderProps> = ({
  cluster,
  sseStatus,
  chaosOpen,
  setChaosOpen,
}) => {
  const isLeader = cluster?.is_leader ?? false;
  const rawLeader = cluster?.leader?.node_id || (isLeader ? cluster?.node_id : '');
  const displayLeader = rawLeader ? rawLeader.replace(/[-_]/g, ' ') : 'Discovering leader';
  const term = cluster?.term || cluster?.fencing_token || 0;
  const chaosEnabled = isChaosFlagEnabled();

  return (
    <header className="sticky top-0 z-40 w-full border-b border-border-subtle bg-surface px-4 py-3 sm:px-6">
      <div className="mx-auto flex flex-wrap items-center justify-between gap-3 max-w-7xl">
        {/* Brand and primary navigation */}
        <div className="flex items-center gap-4 sm:gap-6">
          <NavLink
            to="/cluster"
            className="flex items-center gap-2.5 rounded-md text-text-primary focus-visible:ring-2 focus-visible:ring-accent-focus focus-visible:outline-none"
            aria-label="Orchestra distributed scheduler home"
          >
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-accent-solid text-text-inverse shadow-sm">
              <Activity className="h-5 w-5" aria-hidden="true" />
            </div>
            <div>
              <span className="text-base font-semibold tracking-tight">Orchestra</span>
              <span className="ml-2 rounded bg-surface-elevated px-1.5 py-0.5 text-xs text-text-secondary">
                v1.0
              </span>
            </div>
          </NavLink>

          <nav className="flex items-center gap-1 sm:gap-2" aria-label="Main navigation">
            <NavLink
              to="/cluster"
              className={({ isActive }) =>
                `flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs sm:text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-accent-focus focus-visible:outline-none ${
                  isActive
                    ? 'bg-surface-elevated text-accent-text'
                    : 'text-text-secondary hover:bg-surface-elevated hover:text-text-primary'
                }`
              }
            >
              <LayoutDashboard className="h-4 w-4" aria-hidden="true" />
              <span>Cluster</span>
            </NavLink>

            <NavLink
              to="/workflows"
              className={({ isActive }) =>
                `flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs sm:text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-accent-focus focus-visible:outline-none ${
                  isActive
                    ? 'bg-surface-elevated text-accent-text'
                    : 'text-text-secondary hover:bg-surface-elevated hover:text-text-primary'
                }`
              }
            >
              <GitBranch className="h-4 w-4" aria-hidden="true" />
              <span>Workflows</span>
            </NavLink>

            <NavLink
              to="/submit"
              className={({ isActive }) =>
                `flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs sm:text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-accent-focus focus-visible:outline-none ${
                  isActive
                    ? 'bg-surface-elevated text-accent-text'
                    : 'text-text-secondary hover:bg-surface-elevated hover:text-text-primary'
                }`
              }
            >
              <FilePlus2 className="h-4 w-4" aria-hidden="true" />
              <span>Submit workflow</span>
            </NavLink>
          </nav>
        </div>

        {/* Live cluster status & dev controls */}
        <div className="flex items-center gap-2 sm:gap-3">
          {/* SSE Connection Status */}
          <div
            className={`flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium border ${
              sseStatus === 'connected'
                ? 'bg-success-bg border-success-border text-success-text'
                : sseStatus === 'reconnecting'
                ? 'bg-warning-bg border-warning-border text-warning-text'
                : 'bg-danger-bg border-danger-border text-danger-text'
            }`}
            role="status"
            aria-live="polite"
          >
            {sseStatus === 'connected' && <Wifi className="h-3.5 w-3.5" aria-hidden="true" />}
            {sseStatus === 'reconnecting' && (
              <RefreshCw className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
            )}
            {sseStatus === 'disconnected' && <WifiOff className="h-3.5 w-3.5" aria-hidden="true" />}
            <span className="hidden sm:inline">
              {sseStatus === 'connected'
                ? 'Live connection'
                : sseStatus === 'reconnecting'
                ? 'Reconnecting to cluster...'
                : 'Disconnected'}
            </span>
          </div>

          {/* Active Leader & Fencing Term */}
          <div className="hidden md:flex items-center gap-2 rounded-md border border-border-default bg-surface-subtle px-3 py-1 text-xs">
            <Server className="h-3.5 w-3.5 text-accent-text" aria-hidden="true" />
            <span className="text-text-secondary">Leader:</span>
            <span className="font-mono font-medium text-text-primary capitalize">{displayLeader}</span>
            <span className="rounded bg-surface px-1.5 py-0.5 font-mono text-accent-text">
              Term {term}
            </span>
          </div>

          {/* Dev-only Chaos Control Toggle */}
          {chaosEnabled && (
            <button
              type="button"
              onClick={() => setChaosOpen(prev => !prev)}
              className={`flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-semibold transition-colors focus-visible:ring-2 focus-visible:ring-accent-focus focus-visible:outline-none ${
                chaosOpen
                  ? 'border-danger-border bg-danger-bg text-danger-text'
                  : 'border-border-default bg-surface-elevated text-text-secondary hover:text-text-primary'
              }`}
              title="Open chaos engineering panel"
              aria-expanded={chaosOpen}
            >
              <ShieldAlert className="h-3.5 w-3.5 text-warning-text" aria-hidden="true" />
              <span>Chaos panel</span>
            </button>
          )}
        </div>
      </div>
    </header>
  );
};
