/**
 * Toast context + hook for transient notifications.
 *
 * Usage:
 *   const { toast } = useToast();
 *   toast("Scan dispatched", "success");
 *
 * Variants: success (green), error (red), info (neutral).
 * Auto-dismiss after 4s; manual dismiss via the x button.
 * Voice per design-system.md section 8: calm, no exclamation marks.
 */
import { createContext, useContext } from 'react';

export type ToastVariant = 'success' | 'error' | 'info';

export interface ToastItem {
  id: number;
  message: string;
  variant: ToastVariant;
}

export interface ToastContextValue {
  toast: (message: string, variant?: ToastVariant) => void;
}

export const ToastContext = createContext<ToastContextValue>({
  toast: () => {},
});

export function useToast(): ToastContextValue {
  return useContext(ToastContext);
}
