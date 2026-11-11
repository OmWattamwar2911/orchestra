import React from 'react';

export interface GridSidebarLabelProps {
  label: string;
  index?: string;
  square?: boolean;
  className?: string;
}

export const GridSidebarLabel: React.FC<GridSidebarLabelProps> = ({
  label,
  index,
  square = false,
  className = '',
}) => {
  return (
    <div className={`sticky top-32 space-y-3 ${className}`}>
      {square && (
        <div className="w-4 h-4 bg-jet" aria-hidden="true" />
      )}
      <div className="flex items-center gap-2 font-mono text-xs text-muted">
        {index && <span>{index} /</span>}
        <span className="font-sans font-bold uppercase tracking-widest-poster text-[12px] text-muted">
          {label}
        </span>
      </div>
    </div>
  );
};

export default GridSidebarLabel;
