import React from 'react';
import { PosterButton } from './PosterButton';

export interface ErrorStateProps {
  title?: string;
  message: string;
  remedy?: string;
  onRetry?: () => void;
  className?: string;
}

export const ErrorState: React.FC<ErrorStateProps> = ({
  title = 'SYSTEM ERROR OCCURRED',
  message,
  remedy = 'Check network connectivity or backend orchestrator cluster health and try again.',
  onRetry,
  className = '',
}) => {
  return (
    <div className={`border-2 border-red bg-cream p-6 sm:p-8 space-y-4 ${className}`}>
      <div className="flex items-center gap-2 font-mono text-xs text-red font-bold">
        <span>[!]</span>
        <span>ERROR / FAULT DETECTED</span>
      </div>

      <div className="space-y-1">
        <h3 className="font-display font-bold uppercase tracking-tight text-xl text-jet">
          {title}
        </h3>
        <p className="font-mono text-xs text-red">
          {message}
        </p>
      </div>

      <div className="border-t border-hairline pt-3">
        <p className="text-sm text-deep-gray font-sans leading-relaxed">
          <strong className="font-bold text-jet">Remedy: </strong>
          {remedy}
        </p>
      </div>

      {onRetry && (
        <div className="pt-2">
          <PosterButton variant="secondary" size="sm" onClick={onRetry}>
            RETRY OPERATION
          </PosterButton>
        </div>
      )}
    </div>
  );
};

export default ErrorState;
