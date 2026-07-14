import { useState, useEffect, createContext, useContext, useCallback } from 'react';

interface ToastCtx {
  toast: (msg: string) => void;
}

const ToastCtx = createContext<ToastCtx>({ toast: () => {} });

export function useToast() {
  return useContext(ToastCtx);
}

export function Toaster() {
  const [toasts, setToasts] = useState<{ id: number; msg: string }[]>([]);
  const [counter, setCounter] = useState(0);

  const toast = useCallback((msg: string) => {
    const id = counter;
    setCounter(c => c + 1);
    setToasts(prev => [...prev, { id, msg }]);
    setTimeout(() => {
      setToasts(prev => prev.filter(t => t.id !== id));
    }, 2000);
  }, [counter]);

  // Expose globally for convenience
  useEffect(() => {
    (window as unknown as Record<string, unknown>).__toast = toast;
  }, [toast]);

  return (
    <>
      {toasts.map(t => (
        <div key={t.id} className="toast show">{t.msg}</div>
      ))}
    </>
  );
}
