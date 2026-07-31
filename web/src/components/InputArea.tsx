import { useRef, useCallback } from 'react';
import { useAppState } from '../context/AppContext';
import { t } from '../i18n';

interface Props {
  onSendMessage: (body: {
    message: string; images: string[]; agent_id: number; thread_id?: string; model_id?: string;
  }) => Promise<void>;
  onStop: () => void;
}

export function InputArea({ onSendMessage, onStop }: Props) {
  const { state, dispatch } = useAppState();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const uploadInputRef = useRef<HTMLInputElement>(null);

  const isRunning = state.state === 'thinking' || state.state === 'responding' || state.state === 'tool_calls';

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

    const body: { message: string; images: string[]; agent_id: number; thread_id?: string; model_id?: string } = {
      message: text, images: imgs, agent_id: state.activeAgentId, model_id: state.activeModelId,
    };
    if (state.activeThreadId) body.thread_id = state.activeThreadId;

    onSendMessage(body);
  }, [state.activeThreadId, state.activeAgentId, state.activeModelId, state.hasModels, state.pendingImages, onSendMessage]);

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

  const handleUpload = async (ref: React.RefObject<HTMLInputElement | null>) => {
    const files = ref.current?.files;
    if (!files || files.length === 0) return;
    await uploadFiles(files);
    if (ref.current) ref.current.value = '';
  };

  const uploadFiles = async (fileList: FileList | File[]) => {
    const tid = state.activeThreadId;
    if (!tid) {
      alert('Create a thread first');
      return;
    }
    const form = new FormData();
    form.append('thread_id', tid);
    for (let i = 0; i < fileList.length; i++) {
      const f = fileList[i] as any;
      const relPath = f.webkitRelativePath || f.name;
      form.append('files', f, relPath);
    }
    try {
      const r = await fetch('/api/upload', { method: 'POST', body: form });
      const data = await r.json();
      if (!r.ok) throw new Error(data.error);
      alert(`Uploaded ${data.files?.length || 0} file(s) to workspace/upload/`);
    } catch (e: any) {
      alert('Upload failed: ' + e.message);
    }
  };

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    const items = e.dataTransfer.items;
    if (!items) return;
    const files: File[] = [];
    // Use DataTransferItemList to get files with relative paths for folders
    for (let i = 0; i < items.length; i++) {
      const entry = items[i].webkitGetAsEntry();
      if (entry) {
        await collectFiles(entry, '', files);
      }
    }
    if (files.length > 0) {
      await uploadFiles(files);
    }
  };

  const collectFiles = async (entry: any, path: string, files: File[]): Promise<void> => {
    if (entry.isFile) {
      const file = await new Promise<File>((resolve) => (entry as any).file(resolve));
      // Set webkitRelativePath to preserve directory structure
      Object.defineProperty(file, 'webkitRelativePath', { value: path + entry.name });
      files.push(file);
    } else if (entry.isDirectory) {
      const reader = (entry as any).createReader();
      const entries: any[] = await new Promise((resolve) => reader.readEntries(resolve));
      for (const child of entries) {
        await collectFiles(child, path + entry.name + '/', files);
      }
    }
  };

  return (
    <footer className="input-area" onDrop={handleDrop} onDragOver={e => e.preventDefault()}>
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
        <span className={`status-indicator ${state.state}`} />
        <span className="status-text">
          {state.state === 'thinking' ? t('status.thinking')
           : state.state === 'responding' ? t('status.replying')
           : state.state === 'tool_calls' ? t('status.executing')
           : state.state === 'error' ? t('status.failed')
           : t('status.ready')}
        </span>
      </div>

      <div id="input-capsule">
        {isRunning && (
          <button className="capsule-btn stop-btn" onClick={onStop} title={t('input.stop_title')}>■</button>
        )}
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
        <button className="capsule-btn aux-btn" onClick={() => uploadInputRef.current?.click()} title="Upload files">📁</button>
        <button className="capsule-btn send-btn" onClick={sendMsg} title={t('input.send_title')}>↑</button>
      </div>
      <input ref={fileInputRef} type="file" accept="image/*" multiple hidden onChange={handleFileChange} />
      <input ref={uploadInputRef} type="file" multiple hidden onChange={() => handleUpload(uploadInputRef)} />
    </footer>
  );
}
