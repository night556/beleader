import { useState, createContext, useContext, useCallback } from 'react';

interface ModalOpts {
  title: string;
  body: string;
  wide?: boolean;
  danger?: boolean;
  confirmText?: string;
  cancelText?: string;
  onConfirm?: () => boolean | void;
  onOpen?: () => void;
}

interface ModalCtx {
  open: (opts: ModalOpts) => void;
  close: () => void;
}

const ModalCtx = createContext<ModalCtx>({ open: () => {}, close: () => {} });

export function useModal() {
  return useContext(ModalCtx);
}

export function ModalProvider({ children }: { children: React.ReactNode }) {
  const [opts, setOpts] = useState<ModalOpts | null>(null);
  const [visible, setVisible] = useState(false);

  const open = useCallback((o: ModalOpts) => {
    setOpts(o);
    setVisible(true);
    setTimeout(() => o.onOpen?.(), 50);
  }, []);

  const close = useCallback(() => {
    setVisible(false);
    setTimeout(() => setOpts(null), 200);
  }, []);

  const handleConfirm = () => {
    if (opts?.onConfirm) {
      const result = opts.onConfirm();
      if (result !== false) close();
    } else {
      close();
    }
  };

  return (
    <ModalCtx.Provider value={{ open, close }}>
      {children}
      {opts && (
        <div
          className="modal-backdrop"
          style={{ display: visible ? 'flex' : 'none', opacity: visible ? 1 : 0, transition: 'opacity 0.2s' }}
          onClick={e => { if (e.target === e.currentTarget) close(); }}
        >
          <div className={`modal-dialog ${opts.wide ? 'wide' : ''}`}>
            <div className="modal-head">
              <h3>{opts.title}</h3>
              <button className="modal-close" onClick={close}>✕</button>
            </div>
            <div className="modal-body" dangerouslySetInnerHTML={{ __html: opts.body }} />
            <div className="modal-foot">
              {opts.danger ? (
                <>
                  <button className="modal-btn danger" onClick={handleConfirm}>{opts.confirmText || 'OK'}</button>
                  <button className="modal-btn" onClick={close}>{opts.cancelText || 'Cancel'}</button>
                </>
              ) : (
                <>
                  <button className="modal-btn" onClick={close}>{opts.cancelText || 'Cancel'}</button>
                  <button className="modal-btn primary" onClick={handleConfirm}>{opts.confirmText || 'OK'}</button>
                </>
              )}
            </div>
          </div>
        </div>
      )}
    </ModalCtx.Provider>
  );
}

export { ModalCtx as ModalContext };
// Re-export as Modal for App.tsx compatibility
export const Modal = function ModalWrapper() {
  return null; // Actual modal is rendered by ModalProvider
};
