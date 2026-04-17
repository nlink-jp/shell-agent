import { useState, useEffect, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import 'highlight.js/styles/github-dark.css';
import './App.css';
import {
  SendMessage, SendMessageWithImages, GetTools, GetLLMStatus, ListSessions,
  NewSession, LoadSession, DeleteSession, RenameSession,
  ApproveMITL, RejectMITL, GetPinnedMemories,
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

interface Message {
  role: string;
  content: string;
  timestamp: string;
  images?: string[];
}

interface ToolInfo {
  name: string;
  description: string;
  category: string;
}

interface SessionInfo {
  id: string;
  title: string;
  updated_at: string;
}

interface LLMStatus {
  current_time: string;
  hot_messages: number;
  warm_summaries: number;
  cold_summaries: number;
  tokens_used: number;
  token_limit: number;
}

interface ToolCallRequest {
  id: string;
  name: string;
  arguments: string;
  category: string;
  needs_mitl: boolean;
}

interface ToolResult {
  name: string;
  result: string;
}

interface PinnedMemory {
  fact: string;
  category: string;
  source_time: string;
}

function App() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [streamContent, setStreamContent] = useState('');
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [llmStatus, setLLMStatus] = useState<LLMStatus | null>(null);
  const [sidebarTab, setSidebarTab] = useState<'sessions' | 'tools' | 'status'>('sessions');
  const [mitlRequest, setMitlRequest] = useState<ToolCallRequest | null>(null);
  const [pinnedMemories, setPinnedMemories] = useState<PinnedMemory[]>([]);
  const [pendingImages, setPendingImages] = useState<string[]>([]);
  const composingRef = useRef(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    GetTools().then(setTools);
    ListSessions().then((s) => setSessions(s || []));
    GetPinnedMemories().then((p) => setPinnedMemories(p || []));
    refreshStatus();

    const statusInterval = setInterval(refreshStatus, 5000);

    const offToken = EventsOn('chat:token', (token: string) => {
      setStreamContent(prev => prev + token);
    });

    const offDone = EventsOn('chat:done', () => {
      setStreaming(false);
    });

    const offToolRequest = EventsOn('chat:toolcall_request', (req: ToolCallRequest) => {
      if (req.needs_mitl) {
        setMitlRequest(req);
      }
    });

    const offToolResult = EventsOn('chat:toolresult', (res: ToolResult) => {
      const now = new Date().toLocaleTimeString('ja-JP', { hour12: false });
      setMessages(prev => [...prev, {
        role: 'tool',
        content: `**${res.name}**\n\n${res.result}`,
        timestamp: now,
      }]);
    });

    const offThinking = EventsOn('chat:thinking', () => {
      setStreamContent('');
    });

    const offPinned = EventsOn('chat:pinned_updated', (entries: PinnedMemory[]) => {
      setPinnedMemories(entries || []);
    });

    return () => {
      clearInterval(statusInterval);
      offToken();
      offDone();
      offToolRequest();
      offToolResult();
      offThinking();
      offPinned();
    };
  }, []);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, streamContent, mitlRequest]);

  function refreshStatus() {
    GetLLMStatus().then(setLLMStatus);
  }

  function fileToDataURL(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as string);
      reader.onerror = reject;
      reader.readAsDataURL(file);
    });
  }

  async function addImages(files: FileList | File[]) {
    const newImages: string[] = [];
    for (const file of Array.from(files)) {
      if (file.type.startsWith('image/')) {
        const dataURL = await fileToDataURL(file);
        newImages.push(dataURL);
      }
    }
    if (newImages.length > 0) {
      setPendingImages(prev => [...prev, ...newImages]);
    }
  }

  function handlePaste(e: React.ClipboardEvent) {
    const items = e.clipboardData.items;
    const imageFiles: File[] = [];
    for (const item of Array.from(items)) {
      if (item.type.startsWith('image/')) {
        const file = item.getAsFile();
        if (file) imageFiles.push(file);
      }
    }
    if (imageFiles.length > 0) {
      e.preventDefault();
      addImages(imageFiles);
    }
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault();
    if (e.dataTransfer.files.length > 0) {
      addImages(e.dataTransfer.files);
    }
  }

  function handleDragOver(e: React.DragEvent) {
    e.preventDefault();
  }

  function removeImage(index: number) {
    setPendingImages(prev => prev.filter((_, i) => i !== index));
  }

  async function handleSend() {
    const text = input.trim();
    if ((!text && pendingImages.length === 0) || streaming) return;

    const now = new Date();
    const timestamp = now.toLocaleTimeString('ja-JP', { hour12: false });

    const images = [...pendingImages];
    // Keep data URLs for immediate display in chat
    const userMsg: Message = { role: 'user', content: text || '(image)', timestamp, images: images.length > 0 ? images : undefined };
    setMessages(prev => [...prev, userMsg]);
    setInput('');
    setPendingImages([]);
    setStreaming(true);
    setStreamContent('');

    try {
      const resp = images.length > 0
        ? await SendMessageWithImages(text, images)
        : await SendMessage(text);
      setMessages(prev => [...prev, {
        role: resp.role,
        content: resp.content,
        timestamp: resp.timestamp,
      }]);
      setStreamContent('');
      refreshStatus();
      ListSessions().then((s) => setSessions(s || []));
    } catch (err: any) {
      setMessages(prev => [...prev, {
        role: 'error',
        content: `Error: ${err.message || err}`,
        timestamp,
      }]);
    } finally {
      setStreaming(false);
      setStreamContent('');
    }
  }

  function handleCompositionStart() { composingRef.current = true; }
  function handleCompositionEnd() { setTimeout(() => { composingRef.current = false; }, 50); }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && e.shiftKey && !composingRef.current) {
      e.preventDefault();
      handleSend();
    }
  }

  function handleMITLApprove() {
    setMitlRequest(null);
    ApproveMITL();
  }

  function handleMITLReject() {
    setMitlRequest(null);
    RejectMITL();
  }

  async function handleNewSession() {
    await NewSession();
    setMessages([]);
    ListSessions().then((s) => setSessions(s || []));
    refreshStatus();
  }

  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [editingSession, setEditingSession] = useState<string | null>(null);
  const [editTitle, setEditTitle] = useState('');

  function handleDeleteSession(id: string) {
    setDeleteConfirm(id);
  }

  async function confirmDelete() {
    if (!deleteConfirm) return;
    try {
      await DeleteSession(deleteConfirm);
      ListSessions().then((s) => setSessions(s || []));
    } catch (err) {
      console.error('Failed to delete session:', err);
    } finally {
      setDeleteConfirm(null);
    }
  }

  function startRename(id: string, currentTitle: string) {
    setEditingSession(id);
    setEditTitle(currentTitle);
  }

  async function commitRename() {
    if (!editingSession || !editTitle.trim()) {
      setEditingSession(null);
      return;
    }
    try {
      await RenameSession(editingSession, editTitle.trim());
      ListSessions().then((s) => setSessions(s || []));
    } catch (err) {
      console.error('Failed to rename session:', err);
    } finally {
      setEditingSession(null);
    }
  }

  async function handleLoadSession(id: string) {
    try {
      const msgs = await LoadSession(id);
      setMessages(msgs || []);
      refreshStatus();
    } catch (err) {
      console.error('Failed to load session:', err);
    }
  }

  function formatArgs(argsStr: string): string {
    try {
      const obj = JSON.parse(argsStr);
      return JSON.stringify(obj, null, 2);
    } catch {
      return argsStr;
    }
  }

  return (
    <div className="app">
      <div className="sidebar">
        <div className="sidebar-header">
          <h2>Shell Agent</h2>
          <button className="new-chat-btn" onClick={handleNewSession}>+ New Chat</button>
        </div>

        <div className="sidebar-tabs">
          <button
            className={sidebarTab === 'sessions' ? 'active' : ''}
            onClick={() => setSidebarTab('sessions')}
          >Sessions</button>
          <button
            className={sidebarTab === 'tools' ? 'active' : ''}
            onClick={() => setSidebarTab('tools')}
          >Tools</button>
          <button
            className={sidebarTab === 'status' ? 'active' : ''}
            onClick={() => setSidebarTab('status')}
          >Status</button>
        </div>

        <div className="sidebar-content">
          {sidebarTab === 'sessions' && (
            <div className="session-list">
              {sessions.map(s => (
                <div key={s.id} className="session-item">
                  <div className="session-info" onClick={() => handleLoadSession(s.id)}>
                    {editingSession === s.id ? (
                      <input
                        className="session-title-edit"
                        value={editTitle}
                        onChange={e => setEditTitle(e.target.value)}
                        onBlur={commitRename}
                        onCompositionStart={handleCompositionStart}
                        onCompositionEnd={handleCompositionEnd}
                        onKeyDown={e => { if (e.key === 'Enter' && !composingRef.current) commitRename(); if (e.key === 'Escape') setEditingSession(null); }}
                        onClick={e => e.stopPropagation()}
                        autoFocus
                      />
                    ) : (
                      <span className="session-title" onDoubleClick={(e) => { e.stopPropagation(); startRename(s.id, s.title); }}>{s.title}</span>
                    )}
                    <span className="session-date">{s.updated_at}</span>
                  </div>
                  <button className="session-delete" onClick={(e) => { e.stopPropagation(); handleDeleteSession(s.id); }} title="Delete">&#x2715;</button>
                </div>
              ))}
              {deleteConfirm && (
                <div className="delete-confirm">
                  <span>Delete this session?</span>
                  <div className="delete-confirm-actions">
                    <button className="delete-yes" onClick={confirmDelete}>Delete</button>
                    <button className="delete-no" onClick={() => setDeleteConfirm(null)}>Cancel</button>
                  </div>
                </div>
              )}
              {sessions.length === 0 && <p className="empty">No sessions yet</p>}
            </div>
          )}

          {sidebarTab === 'tools' && (
            <div className="tool-list">
              {tools.map(t => (
                <div key={t.name} className="tool-item">
                  <span className="tool-name">{t.name}</span>
                  <span className={`tool-category ${t.category}`}>{t.category}</span>
                  <p className="tool-desc">{t.description}</p>
                </div>
              ))}
              {tools.length === 0 && <p className="empty">No tools registered</p>}
            </div>
          )}

          {sidebarTab === 'status' && llmStatus && (
            <div className="status-panel">
              <div className="status-item">
                <label>Time</label>
                <span>{llmStatus.current_time}</span>
              </div>
              <div className="status-item">
                <label>Hot</label>
                <span>{llmStatus.hot_messages} messages</span>
              </div>
              <div className="status-item">
                <label>Warm</label>
                <span>{llmStatus.warm_summaries} summaries</span>
              </div>
              <div className="status-item">
                <label>Cold</label>
                <span>{llmStatus.cold_summaries} summaries</span>
              </div>
              <div className="status-item">
                <label>Token Limit</label>
                <span>{llmStatus.token_limit}</span>
              </div>
              <div className="status-item">
                <label>Pinned</label>
                <span>{pinnedMemories.length} facts</span>
              </div>
              {pinnedMemories.length > 0 && (
                <div className="pinned-list">
                  <label className="pinned-header">Remembered Facts</label>
                  {pinnedMemories.map((p, i) => (
                    <div key={i} className="pinned-item">
                      <span className={`pinned-category ${p.category}`}>{p.category}</span>
                      <span className="pinned-fact">{p.fact}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="main">
        <div className="drag-handle" />
        <div className="messages">
          {messages.map((msg, i) => (
            <div key={i} className={`message ${msg.role}`}>
              <div className="message-header">
                <span className="message-role">{msg.role}</span>
                <span className="message-time">{msg.timestamp}</span>
              </div>
              {msg.images && msg.images.length > 0 && (
                <div className="message-images">
                  {msg.images.map((img, j) => (
                    <img key={j} src={img} alt="" className="message-image" />
                  ))}
                </div>
              )}
              <div className="message-content markdown-body">
                <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
                  {msg.content}
                </ReactMarkdown>
              </div>
            </div>
          ))}

          {streaming && !streamContent && !mitlRequest && (
            <div className="message assistant thinking">
              <div className="message-header">
                <span className="message-role">assistant</span>
              </div>
              <div className="message-content">
                <span className="spinner" />
                <span className="thinking-text">Thinking...</span>
              </div>
            </div>
          )}

          {streaming && streamContent && (
            <div className="message assistant streaming">
              <div className="message-header">
                <span className="message-role">assistant</span>
              </div>
              <div className="message-content markdown-body">
                <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
                  {streamContent}
                </ReactMarkdown>
              </div>
            </div>
          )}

          {mitlRequest && (
            <div className="mitl-dialog">
              <div className="mitl-header">
                <span className="mitl-icon">&#9888;</span>
                <span>Tool Approval Required</span>
              </div>
              <div className="mitl-body">
                <div className="mitl-tool-name">
                  <span className="mitl-label">Tool:</span>
                  <code>{mitlRequest.name}</code>
                  <span className={`tool-category ${mitlRequest.category}`}>{mitlRequest.category}</span>
                </div>
                <div className="mitl-args">
                  <span className="mitl-label">Arguments:</span>
                  <pre>{formatArgs(mitlRequest.arguments)}</pre>
                </div>
              </div>
              <div className="mitl-actions">
                <button className="mitl-approve" onClick={handleMITLApprove}>Approve</button>
                <button className="mitl-reject" onClick={handleMITLReject}>Reject</button>
              </div>
            </div>
          )}

          <div ref={messagesEndRef} />
        </div>

        <div className="input-area" onDrop={handleDrop} onDragOver={handleDragOver}>
          {pendingImages.length > 0 && (
            <div className="pending-images">
              {pendingImages.map((img, i) => (
                <div key={i} className="pending-image">
                  <img src={img} alt="" />
                  <button className="pending-image-remove" onClick={() => removeImage(i)}>&#x2715;</button>
                </div>
              ))}
            </div>
          )}
          <div className="input-row">
            <button className="attach-btn" onClick={() => fileInputRef.current?.click()} disabled={streaming} title="Attach image">
              &#x1F4CE;
            </button>
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              multiple
              style={{ display: 'none' }}
              onChange={e => { if (e.target.files) addImages(e.target.files); e.target.value = ''; }}
            />
            <textarea
              ref={inputRef}
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              onPaste={handlePaste}
              onCompositionStart={handleCompositionStart}
              onCompositionEnd={handleCompositionEnd}
              placeholder="Type a message... (Shift+Enter to send, Enter for newline)"
              disabled={streaming}
              rows={3}
            />
            <button onClick={handleSend} disabled={streaming || (!input.trim() && pendingImages.length === 0)}>
              {streaming ? '...' : 'Send'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

export default App;
