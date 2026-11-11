import React from 'react';

export interface SkeletonProps {
  className?: string;
  height?: string | number;
  width?: string | number;
}

export const Skeleton: React.FC<SkeletonProps> = ({
  className = '',
  height,
  width,
}) => {
  return (
    <div
      style={{ height, width }}
      className={`border border-hairline bg-transparent animate-pulse ${className}`}
      aria-hidden="true"
    />
  );
};

export default Skeleton;
