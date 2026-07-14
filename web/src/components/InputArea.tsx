import { useRef, useCallback, useState } from 'react';
import { useAppState } from '../context/AppContext';
import { client } from '../api/client';
import { t } from '../i18n';

export function InputArea() {
  const { state, dispatch } = useAppState();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [statusText] = useState(t('status.ready'));

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

    // Show user message
    if (text) {
      dispatch({
        type: 'PUSH_TIMELINE_ITEM',
        item: { id: '', icon: '👤', type: 'user', label: 'You', content: text, status: 'done', time: Date.now() },
      });
    }

    const body: { message: string; images: string[]; agent_id: number; thread_id?: string } = {
      message: text, images: imgs, agent_id: state.activeAgentId,
    };
    if (state.activeThreadId) body.thread_id = state.activeThreadId;

    client.sendChat(body).then(d => {
      if (d.thread_id && !state.activeThreadId) {
        dispatch({ type: 'SET_ACTIVE_THREAD', threadId: d.thread_id });
        // Refresh thread list
        client.listThreads().then(threads => {
          dispatch({ type: 'SET_THREADS', threads });
        });
      }
    }).catch(err => console.error('chat error:', err));
  }, [state.activeThreadId, state.activeAgentId, state.hasModels, state.pendingImages]);

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
    if (!state.activeThreadId) return;
    client.pauseThread(state.activeThreadId).catch(err => console.error('stop error:', err));
  };

  const clearContext = () => {
    if (!state.activeThreadId) return;
    if (!confirm('Clear the conversation context for this thread?')) return;
    client.pauseThread(state.activeThreadId).then(() => {
      dispatch({ type: 'CLEAR_TIMELINE' });
    }).catch(err => alert('Clear failed: ' + err.message));
  };

  return (
    <footer className="input-area">
      {/* Image preview */}
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
        <span className="status-text">{statusText}</span>
        {state.activeThreadId && (
          <button onClick={clearContext} style={{ background: 'none', border: 'none', color: 'var(--text-dim)', fontSize: 11, marginLeft: 'auto', cursor: 'pointer' }}>
            {t('ctx.clear')}
          </button>
        )}
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
