import React, { useState } from 'react';
import { Plus, Minus } from 'lucide-react';

export interface TypographicListItemProps {
  index: string;
  title: string;
  subtitle?: string;
  explanation?: string;
  size?: 'landing' | 'dashboard';
  active?: boolean;
  onClick?: () => void;
  className?: string;
}

export const TypographicListItem: React.FC<TypographicListItemProps> = ({
  index,
  title,
  subtitle,
  explanation,
  size = 'landing',
  active = false,
  onClick,
  className = '',
}) => {
  const [expanded, setExpanded] = useState(false);

  const handleClick = () => {
    if (explanation) {
      setExpanded(prev => !prev);
    }
    if (onClick) {
      onClick();
    }
  };

  const titleSizeClass = size === 'landing' ? 'text-3xl sm:text-4xl lg:text-5xl' : 'text-xl sm:text-2xl';

  return (
    <div
      onClick={handleClick}
      className={`group border-t border-hairline py-6 transition-colors duration-300 cursor-pointer ${className}`}
      role={explanation ? 'button' : undefined}
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          handleClick();
        }
      }}
    >
      <div className="flex items-baseline justify-between gap-4">
        <div className="flex items-baseline gap-4 sm:gap-8">
          <span className="font-mono text-xs sm:text-sm text-muted">
            {index}
          </span>
          <div>
            <h3
              className={`font-display font-bold uppercase tracking-tight text-jet transition-colors duration-300 group-hover:text-cobalt ${
                active ? 'text-cobalt' : ''
              } ${titleSizeClass}`}
            >
              {title}
            </h3>
            {subtitle && (
              <p className="mt-1 font-mono text-xs text-muted-strong">
                {subtitle}
              </p>
            )}
          </div>
        </div>

        {explanation && (
          <div className="text-muted group-hover:text-cobalt transition-colors duration-300">
            {expanded ? <Minus className="h-5 w-5" /> : <Plus className="h-5 w-5" />}
          </div>
        )}
      </div>

      {expanded && explanation && (
        <div className="mt-4 pl-10 sm:pl-16 text-sm text-deep-gray leading-relaxed max-w-2xl font-sans">
          {explanation}
        </div>
      )}
    </div>
  );
};

export default TypographicListItem;
