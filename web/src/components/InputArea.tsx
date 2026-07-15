import { useRef, useCallback } from 'react';
import { useAppState } from '../context/AppContext';
import { t } from '../i18n';

interface Props {
  onSendMessage: (body: {
    message: string; images: string[]; agent_id: number; thread_id?: string;
  }) => Promise<void>;
  onStop: () => void;
}

export function InputArea({ onSendMessage, onStop }: Props) {
  const { state, dispatch } = useAppState();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const sendMsg = useCallback(() => {
    const text = textareaRef.current?.value.trim() || '';
    const imgs = state.pendingImages.slice();
    dispatch({ type: 'CLEAR_PENDING_IMAGES' });

    if (!text && imgs.length === 0) return;
    if (!state.hasModels) {
      alert('No model configured. Add one in Settings.');
      return;
    }
    if (!state.activeAgentId) {
      alert('No agent available.');
      return;
    }

    if (textareaRef.current) {
      textareaRef.current.value = '';
      textareaRef.current.style.height = 'auto';
    }

    const body: { message: string; images: string[]; agent_id: number; thread_id?: string } = {
      message: text, images: imgs, agent_id: state.activeAgentId,
    };
    if (state.activeThreadId) body.thread_id = state.activeThreadId;

    onSendMessage(body);
  }, [state.activeThreadId, state.activeAgentId, state.hasModels, state.pendingImages, onSendMessage]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMsg();
    }
  };

  const handleInput = () => {
    const ta = textareaRef.current;
    if (ta) {
      ta.style.height = 'auto';
      ta.style.height = Math.min(ta.scrollHeight, 120) + 'px';
    }
  };

  const handlePaste = (e: React.ClipboardEvent) => {
    const items = e.clipboardData?.items;
    if (!items) return;
    for (let i = 0; i < items.length; i++) {
      if (items[i].type.startsWith('image/')) {
        e.preventDefault();
        const blob = items[i].getAsFile();
        if (!blob) continue;
        const reader = new FileReader();
        reader.onload = (ev) => {
          if (ev.target?.result) {
            dispatch({ type: 'ADD_PENDING_IMAGE', image: ev.target.result as string });
          }
        };
        reader.readAsDataURL(blob);
      }
    }
  };

  const handleFileChange = () => {
    const files = fileInputRef.current?.files;
    if (!files) return;
    for (let i = 0; i < files.length; i++) {
      const reader = new FileReader();
      reader.onload = (ev) => {
        if (ev.target?.result) {
          dispatch({ type: 'ADD_PENDING_IMAGE', image: ev.target.result as string });
        }
      };
      reader.readAsDataURL(files[i]);
    }
    if (fileInputRef.current) fileInputRef.current.value = '';
  };

  const stopSession = () => {
    onStop();
  };

  return (
    <footer className="input-area">
      {state.pendingImages.length > 0 && (
        <div className="img-preview">
          {state.pendingImages.map((img, i) => (
            <div key={i} style={{ position: 'relative' }}>
              <img src={img} alt="" />
              <button
                className="img-preview-remove"
                onClick={() => dispatch({
                  type: 'SET_PENDING_IMAGES',
                  images: state.pendingImages.filter((_, j) => j !== i),
                })}
                style={{ position: 'absolute', top: -6, right: -6 }}
              >×</button>
            </div>
          ))}
        </div>
      )}

      <div className="status-bar">
        <span className="status-indicator" />
        <span className="status-text">{t('status.ready')}</span>
      </div>

      <div id="input-capsule">
        <button className="capsule-btn stop-btn" onClick={stopSession} title={t('input.stop_title')}>■</button>
        <textarea
          id="msg-input"
          ref={textareaRef}
          rows={1}
          placeholder={t('input.placeholder')}
          onKeyDown={handleKeyDown}
          onInput={handleInput}
          onPaste={handlePaste}
        />
        <button className="capsule-btn aux-btn" onClick={() => fileInputRef.current?.click()} title={t('input.upload_title')}>📷</button>
        <button className="capsule-btn send-btn" onClick={sendMsg} title={t('input.send_title')}>↑</button>
      </div>
      <input ref={fileInputRef} type="file" accept="image/*" multiple hidden onChange={handleFileChange} />
    </footer>
  );
}
