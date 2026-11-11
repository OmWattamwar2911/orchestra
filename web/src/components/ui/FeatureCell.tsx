import React from 'react';

export interface FeatureCellProps {
  index: string;
  title: string;
  description: string;
  className?: string;
}

export const FeatureCell: React.FC<FeatureCellProps> = ({
  index,
  title,
  description,
  className = '',
}) => {
  return (
    <div
      className={`border border-hairline p-6 sm:p-8 flex flex-col justify-between transition-colors duration-300 hover:bg-[rgba(255,255,255,0.2)] ${className}`}
    >
      <div className="font-mono text-xs text-muted mb-6">
        {index}
      </div>
      <div className="space-y-3">
        <h3 className="font-display font-bold uppercase tracking-tight text-lg sm:text-xl text-jet">
          {title}
        </h3>
        <p className="font-sans text-sm text-deep-gray leading-relaxed">
          {description}
        </p>
      </div>
    </div>
  );
};

export default FeatureCell;
