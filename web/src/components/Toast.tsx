/**
 * ToastProvider + Toast UI component.
 *
 * Wraps the app in App.tsx. Renders a fixed-position stack at the
 * bottom-right of the viewport. Each toast auto-dismisses after 4s
 * or can be dismissed manually via the x button.
 *
 * Styling uses design-system.md tokens for status colors.
 */
import { useState, useCallback, useRef, type ReactNode } from 'react';
import { ToastContext, type ToastItem, type ToastVariant } from '../lib/toast';

const AUTO_DISMISS_MS = 4000;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const nextId = useRef(0);

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const toast = useCallback(
    (message: string, variant: ToastVariant = 'info') => {
      const id = ++nextId.current;
      setToasts((prev) => [...prev, { id, message, variant }]);
      setTimeout(() => dismiss(id), AUTO_DISMISS_MS);
    },
    [dismiss],
  );

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      <div style={containerStyle}>
        {toasts.map((t) => (
          <div key={t.id} style={{ ...itemStyle, ...variantStyle(t.variant) }}>
            <span>{t.message}</span>
            <button
              type="button"
              onClick={() => dismiss(t.id)}
              style={closeStyle}
              aria-label="Dismiss"
            >
              &times;
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

// ---------- inline styles (design-system tokens) ----------

const containerStyle: React.CSSProperties = {
  position: 'fixed',
  bottom: 16,
  right: 16,
  display: 'flex',
  flexDirection: 'column',
  gap: 8,
  zIndex: 200,
  pointerEvents: 'none',
};

const itemStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 12,
  padding: '10px 16px',
  borderRadius: 6,
  fontSize: 14,
  fontWeight: 500,
  boxShadow: '0 4px 6px rgba(0,0,0,.07)',
  pointerEvents: 'auto',
  maxWidth: 380,
  lineHeight: 1.4,
};

function variantStyle(v: ToastVariant): React.CSSProperties {
  switch (v) {
    case 'success':
      return {
        background: 'var(--ss-success-bg, #d1fae5)',
        color: 'var(--ss-success, #10b981)',
        border: '1px solid color-mix(in srgb, var(--ss-success, #10b981) 30%, transparent)',
      };
    case 'error':
      return {
        background: 'var(--ss-danger-bg, #fee2e2)',
        color: 'var(--ss-danger, #ef4444)',
        border: '1px solid color-mix(in srgb, var(--ss-danger, #ef4444) 30%, transparent)',
      };
    case 'info':
      return {
        background: 'var(--ss-info-bg, #cffafe)',
        color: 'var(--ss-info, #06b6d4)',
        border: '1px solid color-mix(in srgb, var(--ss-info, #06b6d4) 30%, transparent)',
      };
  }
}

const closeStyle: React.CSSProperties = {
  background: 'none',
  border: 'none',
  cursor: 'pointer',
  fontSize: 18,
  lineHeight: 1,
  color: 'inherit',
  opacity: 0.7,
  padding: 0,
  marginLeft: 'auto',
};
