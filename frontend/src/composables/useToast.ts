import { ref } from 'vue';
import type { ToastType } from '../types/ui';

interface Toast {
  id: string;
  message: string;
  type: ToastType;
  duration?: number;
}

const toasts = ref<Toast[]>([]);

export function useToast() {
  const show = (message: string, type: ToastType = 'info', duration = 3000) => {
    const id = Math.random().toString(36).substring(2, 9);
    const toast: Toast = { id, message, type, duration };
    toasts.value.push(toast);

    if (duration > 0) {
      setTimeout(() => {
        remove(id);
      }, duration);
    }
  };

  const remove = (id: string) => {
    toasts.value = toasts.value.filter((t) => t.id !== id);
  };

  const success = (msg: string) => show(msg, 'success');
  const error = (msg: string) => show(msg, 'error', 5000);
  const info = (msg: string) => show(msg, 'info');
  const warn = (msg: string) => show(msg, 'warning');

  return {
    toasts,
    show,
    success,
    error,
    info,
    warn,
    remove,
  };
}
