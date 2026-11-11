import React, { createContext, useContext, useState, useCallback } from 'react';
import { X } from 'lucide-react';

export interface Toast {
  id: string;
  message: string;
}

interface ToastContextValue {
  showToast: (message: string, type?: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const removeToast = useCallback((id: string) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  const showToast = useCallback((message: string, _type?: string) => {
    const id = Math.random().toString(36).substring(2, 9);
    setToasts(prev => [...prev, { id, message }]);
    setTimeout(() => {
      removeToast(id);
    }, 4000);
  }, [removeToast]);

  return (
    <ToastContext.Provider value={{ showToast }}>
      {children}
      <aside
        aria-label="Notifications"
        aria-live="polite"
        className="fixed bottom-6 right-6 z-50 flex flex-col gap-2 max-w-md pointer-events-none"
      >
        {toasts.map(toast => (
          <div
            key={toast.id}
            role="status"
            className="pointer-events-auto flex items-center justify-between gap-4 border border-hairline bg-jet text-cream px-5 py-3 text-xs font-mono tracking-wider shadow-none transition-colors duration-300"
          >
            <div className="flex items-center gap-2">
              <span className="text-cobalt">■</span>
              <span>{toast.message}</span>
            </div>
            <button
              type="button"
              onClick={() => removeToast(toast.id)}
              className="text-cream hover:text-cobalt p-1 focus-visible:outline-none"
              aria-label="Dismiss notification"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
        ))}
      </aside>
    </ToastContext.Provider>
  );
};

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return context;
}

export default ToastProvider;
