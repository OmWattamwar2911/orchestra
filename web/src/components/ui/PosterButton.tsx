import React from 'react';

export interface PosterButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'link' | 'danger';
  size?: 'sm' | 'md' | 'lg';
  loading?: boolean;
}

export const PosterButton: React.FC<PosterButtonProps> = ({
  variant = 'primary',
  size = 'md',
  loading = false,
  children,
  disabled,
  className = '',
  ...props
}) => {
  const base = "inline-flex items-center justify-center font-bold uppercase tracking-wider select-none focus-visible:outline-none transition-colors duration-300";

  const sizeClasses = {
    sm: "px-4 py-2 text-xs",
    md: "px-8 py-4 text-sm", // 16px 32px padding, 14px font
    lg: "px-10 py-5 text-base",
  }[size];

  const variantClasses = {
    primary: "bg-cobalt text-cream hover:bg-jet disabled:opacity-50 disabled:cursor-not-allowed",
    secondary: "bg-jet text-cream hover:bg-cobalt disabled:opacity-50 disabled:cursor-not-allowed",
    danger: "bg-red text-cream hover:bg-jet disabled:opacity-50 disabled:cursor-not-allowed",
    link: "bg-transparent text-jet border-b-2 border-jet hover:text-cobalt hover:border-cobalt px-0 py-1 text-sm tracking-wider disabled:opacity-50",
  }[variant];

  return (
    <button
      disabled={disabled || loading}
      className={`${base} ${variant === 'link' ? '' : sizeClasses} ${variantClasses} ${className}`}
      {...props}
    >
      {loading ? (
        <span className="font-mono text-xs animate-pulse">[ RUNNING... ]</span>
      ) : (
        children
      )}
    </button>
  );
};

export default PosterButton;
