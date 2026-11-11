import React from 'react';
import { PosterButton } from './PosterButton';

export interface EmptyStateProps {
  title?: string;
  message: string;
  actionText?: string;
  onAction?: () => void;
  className?: string;
}

export const EmptyState: React.FC<EmptyStateProps> = ({
  title = 'NO DATA AVAILABLE',
  message,
  actionText,
  onAction,
  className = '',
}) => {
  return (
    <div className={`border border-hairline p-8 sm:p-12 text-center space-y-4 ${className}`}>
      <div className="font-mono text-xs text-muted">[ EMPTY STATE ]</div>
      <h3 className="font-display font-bold uppercase tracking-tight text-xl text-jet">
        {title}
      </h3>
      <p className="text-sm text-deep-gray max-w-md mx-auto leading-relaxed font-sans">
        {message}
      </p>
      {actionText && onAction && (
        <div className="pt-2">
          <PosterButton variant="secondary" size="sm" onClick={onAction}>
            {actionText}
          </PosterButton>
        </div>
      )}
    </div>
  );
};

export default EmptyState;
