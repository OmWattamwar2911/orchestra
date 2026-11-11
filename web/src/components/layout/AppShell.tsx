import React, { useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { Menu, X } from 'lucide-react';
import { StickyNav } from './StickyNav';
import { useCluster } from '../../context/ClusterContext';
import { PRODUCT_NAME } from '../../config';

export const AppShell: React.FC = () => {
  const { cluster } = useCluster();
  const location = useLocation();
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  const term = cluster?.term || cluster?.fencing_token || 1;
  const isLeader = cluster?.is_leader ?? false;
  const rawLeader = cluster?.leader?.node_id || (isLeader ? cluster?.node_id : '');
  const displayLeader = rawLeader ? rawLeader.replace(/[-_]/g, '-') : 'orchestrator-1';
  const workerCount = cluster?.worker_count || 5;

  const navItems = [
    { to: '/app', label: 'Cluster', index: '001', end: true },
    { to: '/app/workflows', label: 'Workflows', index: '002' },
    { to: '/app/submit', label: 'Submit', index: '003' },
    { to: '/app/resilience', label: 'Resilience', index: '004' },
    { to: '/app/docs', label: 'Docs', index: '005' },
  ];

  // Determine current active section label
  const currentSection = navItems.find(item => 
    item.end ? location.pathname === item.to : location.pathname.startsWith(item.to)
  )?.label.toUpperCase() || 'DASHBOARD';

  return (
    <div className="min-h-screen bg-cream text-jet flex flex-col font-sans">
      {/* 80px Sticky Nav */}
      <StickyNav mode="app" />

      {/* Main 12-Column Grid Container */}
      <div className="flex-1 w-full max-w-[1600px] mx-auto grid grid-cols-12 min-h-[calc(100vh-80px)]">
        {/* Mobile Section Header & Hamburger Toggle */}
        <div className="lg:hidden col-span-12 border-b border-hairline px-6 py-4 flex items-center justify-between bg-cream">
          <div className="flex items-center gap-2 font-mono text-xs text-muted uppercase">
            <span>[ SECTION ]</span>
            <span className="font-bold text-jet">{currentSection}</span>
          </div>
          <button
            type="button"
            onClick={() => setMobileMenuOpen(prev => !prev)}
            className="p-1 text-jet hover:text-cobalt focus-visible:outline-none"
            aria-label="Toggle navigation menu"
          >
            {mobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
          </button>
        </div>

        {/* Mobile Navigation Drawer */}
        {mobileMenuOpen && (
          <nav className="lg:hidden col-span-12 border-b border-hairline bg-cream px-6 py-4 space-y-2">
            {navItems.map(item => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                onClick={() => setMobileMenuOpen(false)}
                className={({ isActive }) =>
                  `flex items-center gap-3 py-3 border-b border-hairline font-mono text-sm uppercase tracking-wider ${
                    isActive ? 'text-cobalt font-bold' : 'text-jet hover:text-cobalt'
                  }`
                }
              >
                <span className="text-muted text-xs">{item.index}</span>
                <span>{item.label}</span>
              </NavLink>
            ))}
          </nav>
        )}

        {/* Desktop Sticky Sidebar: Cols 1-3 */}
        <aside className="hidden lg:flex col-span-3 border-r border-hairline p-8 flex-col justify-between sticky top-20 h-[calc(100vh-80px)]">
          <div className="space-y-8">
            {/* Sidebar Label on Top */}
            <div className="space-y-3">
              <div className="w-4 h-4 bg-jet" aria-hidden="true" />
              <div className="font-mono text-xs uppercase tracking-widest text-muted">
                DASHBOARD / {currentSection}
              </div>
            </div>

            {/* Nav Items as TypographicListItems */}
            <nav className="space-y-0" aria-label="Dashboard navigation">
              {navItems.map(item => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.end}
                  className={({ isActive }) =>
                    `group block border-t border-hairline py-4 transition-colors duration-300 focus-visible:outline-none ${
                      isActive ? 'text-cobalt' : 'text-jet hover:text-cobalt'
                    }`
                  }
                >
                  <div className="flex items-baseline gap-4">
                    <span className="font-mono text-xs text-muted">
                      {item.index}
                    </span>
                    <span className="font-display font-bold uppercase tracking-tight text-xl">
                      {item.label}
                    </span>
                  </div>
                </NavLink>
              ))}
              <div className="border-t border-hairline" />
            </nav>
          </div>

          {/* Bottom Sidebar Cluster Metadata Block */}
          <div className="border border-hairline p-4 space-y-2 font-mono text-xs bg-cream">
            <div className="flex justify-between text-muted">
              <span>LEADER:</span>
              <span className="text-jet font-bold">{displayLeader}</span>
            </div>
            <div className="flex justify-between text-muted">
              <span>TERM:</span>
              <span className="text-jet font-bold">{term}</span>
            </div>
            <div className="flex justify-between text-muted">
              <span>WORKERS:</span>
              <span className="text-cobalt font-bold">{workerCount} ACTIVE</span>
            </div>
            <div className="border-t border-hairline pt-2 flex justify-between text-[10px] text-muted-strong">
              <span>BUILD:</span>
              <span className="font-bold text-jet">{typeof __COMMIT_HASH__ !== 'undefined' ? __COMMIT_HASH__ : 'dev'}</span>
            </div>
          </div>
        </aside>

        {/* Main Content Area: Cols 4-12 */}
        <main className="col-span-12 lg:col-span-9 flex flex-col min-w-0">
          <Outlet />
          {/* App Footer Stamp */}
          <footer className="w-full border-t border-hairline p-6 font-mono text-xs text-muted-strong flex flex-col sm:flex-row sm:items-center justify-between gap-2 bg-cream">
            <div className="flex items-center gap-2">
              <span className="w-2 h-2 bg-cobalt inline-block" />
              <span>{PRODUCT_NAME} // POSTER MODERNIST SPEC</span>
            </div>
            <div>
              BUILD: <strong className="text-jet">{typeof __COMMIT_HASH__ !== 'undefined' ? __COMMIT_HASH__ : 'dev'}</strong> ({typeof __BUILD_TIME__ !== 'undefined' ? __BUILD_TIME__.slice(0, 16).replace('T', ' ') : ''})
            </div>
          </footer>
        </main>
      </div>
    </div>
  );
};

export default AppShell;
