import React from 'react';

export interface TabItem {
  id: string;
  label: string;
  badge?: number | string;
  icon?: React.ReactNode;
}

export interface TabsProps {
  tabs: TabItem[];
  activeTab: string;
  onChange: (id: string) => void;
  className?: string;
}

export const Tabs: React.FC<TabsProps> = ({
  tabs,
  activeTab,
  onChange,
  className = '',
}) => {
  return (
    <div
      role="tablist"
      aria-orientation="horizontal"
      className={`flex items-center gap-6 border-b border-hairline ${className}`}
    >
      {tabs.map(tab => {
        const isActive = activeTab === tab.id;
        return (
          <button
            key={tab.id}
            role="tab"
            aria-selected={isActive}
            tabIndex={isActive ? 0 : -1}
            onClick={() => onChange(tab.id)}
            className={`flex items-center gap-2 py-3 text-xs font-mono font-bold uppercase tracking-wider transition-colors duration-300 -mb-px focus-visible:outline-none ${
              isActive
                ? 'border-b-2 border-cobalt text-cobalt'
                : 'border-b-2 border-transparent text-muted hover:text-jet'
            }`}
          >
            {tab.icon && <span>{tab.icon}</span>}
            <span>{tab.label}</span>
            {tab.badge !== undefined && (
              <span className={`px-1 py-0.2 text-[10px] ${isActive ? 'bg-cobalt text-cream' : 'border border-hairline text-muted'}`}>
                {tab.badge}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
};

export default Tabs;
