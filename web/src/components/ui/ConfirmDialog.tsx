import React from 'react';
import { X } from 'lucide-react';
import { PosterButton } from './PosterButton';

export interface ConfirmDialogProps {
  isOpen: boolean;
  title: string;
  description?: string;
  message?: string;
  confirmLabel?: string;
  confirmText?: string;
  cancelLabel?: string;
  confirmVariant?: 'danger' | 'primary' | 'secondary';
  variant?: 'danger' | 'primary';
  onConfirm: () => void;
  onCancel: () => void;
}

export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  isOpen,
  title,
  description,
  message,
  confirmLabel,
  confirmText,
  cancelLabel = 'CANCEL',
  confirmVariant = 'danger',
  onConfirm,
  onCancel,
}) => {
  if (!isOpen) return null;

  const bodyText = description || message || '';
  const actionText = confirmText || confirmLabel || 'CONFIRM';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      {/* Flat Backdrop */}
      <div
        onClick={onCancel}
        className="fixed inset-0 bg-jet/60"
      />

      {/* Flat Modal Panel */}
      <div
        className="relative z-10 w-full max-w-lg border border-hairline bg-cream p-8 shadow-none"
        role="alertdialog"
        aria-modal="true"
      >
        <div className="flex items-start justify-between gap-4 mb-4">
          <div className="space-y-1">
            <span className="font-mono text-xs text-muted">[ CONFIRM ACTION ]</span>
            <h3 className="font-display font-bold uppercase tracking-tight text-xl text-jet">
              {title}
            </h3>
          </div>
          <button
            type="button"
            onClick={onCancel}
            className="text-jet hover:text-cobalt p-1 focus-visible:outline-none"
            aria-label="Close dialog"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <p className="text-sm text-deep-gray leading-relaxed font-sans mb-8">
          {bodyText}
        </p>

        <div className="flex items-center justify-end gap-4 border-t border-hairline pt-6">
          <PosterButton variant="link" onClick={onCancel}>
            {cancelLabel}
          </PosterButton>
          <PosterButton variant={confirmVariant} size="sm" onClick={onConfirm}>
            {actionText}
          </PosterButton>
        </div>
      </div>
    </div>
  );
};

export default ConfirmDialog;
