import React from 'react';

export interface PanelProps extends React.HTMLAttributes<HTMLDivElement> {
  title?: string;
  description?: string;
  index?: string;
}

export const Panel: React.FC<PanelProps> = ({
  title,
  description,
  index,
  className = '',
  children,
  ...props
}) => {
  return (
    <div className={`border border-hairline bg-cream p-6 sm:p-8 ${className}`} {...props}>
      {(title || description || index) && (
        <div className="mb-6 space-y-2 border-b border-hairline pb-4">
          {index && (
            <div className="font-mono text-xs text-muted">
              {index}
            </div>
          )}
          {title && (
            <h3 className="font-display font-bold uppercase tracking-tight text-lg sm:text-xl text-jet">
              {title}
            </h3>
          )}
          {description && (
            <p className="text-sm text-deep-gray font-sans leading-relaxed">
              {description}
            </p>
          )}
        </div>
      )}
      {children}
    </div>
  );
};

export default Panel;
