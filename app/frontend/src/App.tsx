import { useState, useEffect, useRef, useCallback } from 'react';
import ChatInput from './ChatInput';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import 'highlight.js/styles/github-dark.css';
import './App.css';
import {
  SendMessage, SendMessageWithImages, GetTools, GetLLMStatus, ListSessions,
  NewSession, LoadSession, DeleteSession, RenameSession,
  ApproveMITL, RejectMITL, GetPinnedMemories,
  UpdatePinnedMemory, DeletePinnedMemory,
  GetConfig, SaveConfig,
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

interface Message {
  role: string;
  content: string;
  timestamp: string;
  imageIds?: string[]; // keys into imageCache ref
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
  image?: string;
}

interface PinnedMemory {
  fact: string;
  native_fact: string;
  category: string;
  source_time: string;
  created_at: string;
}

function App() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [streamContent, setStreamContent] = useState('');
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [llmStatus, setLLMStatus] = useState<LLMStatus | null>(null);
  const [sidebarTab, setSidebarTab] = useState<'sessions' | 'tools' | 'status'>('sessions');
  const [mitlRequest, setMitlRequest] = useState<ToolCallRequest | null>(null);
  const [executingTool, setExecutingTool] = useState<string | null>(null);
  const [pinnedMemories, setPinnedMemories] = useState<PinnedMemory[]>([]);
  const [showSettings, setShowSettings] = useState(false);
  const [settings, setSettings] = useState<any>(null);
  const [lightboxImage, setLightboxImage] = useState<string | null>(null);
  const composingRef = useRef(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const imageCacheRef = useRef<Record<string, string>>({});
  let nextImageId = useRef(0);

  function cacheImage(dataURL: string): string {
    const id = `img-${nextImageId.current++}`;
    imageCacheRef.current[id] = dataURL;
    return id;
  }

  function getCachedImage(id: string): string {
    return imageCacheRef.current[id] || '';
  }
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Apply theme, restore last session, and fetch location on load
  useEffect(() => {
    GetConfig().then((cfg: any) => {
      if (cfg?.theme) {
        document.documentElement.setAttribute('data-theme', cfg.theme);
      }
      if (cfg?.startup_mode === 'last' && cfg?.last_session) {
        LoadSession(cfg.last_session).then((msgs) => {
          if (msgs) setMessages(convertLoadedMessages(msgs));
        }).catch(() => {});
      }
      // Location is fetched by Go backend via CoreLocation helper
    });
  }, []);

  useEffect(() => {
    GetTools().then(setTools);
    ListSessions().then((s) => setSessions(s || []));
    GetPinnedMemories().then((p) => setPinnedMemories(p || []));
    refreshStatus();

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

    const offToolExecuting = EventsOn('chat:tool_executing', (info: {name: string}) => {
      setExecutingTool(info.name);
    });

    const offToolResult = EventsOn('chat:toolresult', (res: ToolResult) => {
      setExecutingTool(null);
      const now = new Date().toLocaleTimeString('ja-JP', { hour12: false });
      setMessages(prev => [...prev, {
        role: 'tool',
        content: `**${res.name}**\n\n${res.result}`,
        timestamp: now,
        imageIds: res.image ? [cacheImage(res.image)] : undefined,
      }]);
    });

    const offThinking = EventsOn('chat:thinking', () => {
      setStreamContent('');
    });

    const offPinned = EventsOn('chat:pinned_updated', (entries: PinnedMemory[]) => {
      setPinnedMemories(entries || []);
    });

    return () => {
      offToken();
      offDone();
      offToolRequest();
      offToolExecuting();
      offToolResult();
      offThinking();
      offPinned();
    };
  }, []);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, streamContent]);

  function refreshStatus() {
    GetLLMStatus().then(setLLMStatus);
  }

  function handleCompositionStart() { composingRef.current = true; }
  function handleCompositionEnd() { setTimeout(() => { composingRef.current = false; }, 50); }

  const handleSend = useCallback(async (text: string, images: string[]) => {
    const now = new Date();
    const timestamp = now.toLocaleTimeString('ja-JP', { hour12: false });

    const imageIds = images.length > 0 ? images.map(img => cacheImage(img)) : undefined;
    const userMsg: Message = { role: 'user', content: text || '(image)', timestamp, imageIds };
    setMessages(prev => [...prev, userMsg]);
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
  }, []);

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
  const [editingPinned, setEditingPinned] = useState<number | null>(null);
  const [editPinnedFact, setEditPinnedFact] = useState('');
  const [editPinnedNative, setEditPinnedNative] = useState('');
  const [editPinnedCategory, setEditPinnedCategory] = useState('');
  const [deletePinnedConfirm, setDeletePinnedConfirm] = useState<number | null>(null);

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

  function convertLoadedMessages(msgs: any[]): Message[] {
    return msgs.map(m => ({
      role: m.role,
      content: m.content,
      timestamp: m.timestamp,
      imageIds: m.images ? m.images.map((img: string) => cacheImage(img)) : undefined,
    }));
  }

  async function handleLoadSession(id: string) {
    try {
      const msgs = await LoadSession(id);
      setMessages(convertLoadedMessages(msgs || []));
      refreshStatus();
    } catch (err) {
      console.error('Failed to load session:', err);
    }
  }

  async function openSettings() {
    try {
      const cfg = await GetConfig();
      setSettings(cfg);
      setShowSettings(true);
    } catch (err) {
      console.error('Failed to load config:', err);
    }
  }

  async function handleSaveSettings() {
    if (!settings) return;
    try {
      await SaveConfig(JSON.stringify(settings));
      document.documentElement.setAttribute('data-theme', settings.theme || 'dark');
      setShowSettings(false);
    } catch (err: any) {
      alert('Failed to save: ' + (err.message || err));
    }
  }

  function updateSetting(section: string, key: string, value: any) {
    setSettings((prev: any) => ({
      ...prev,
      [section]: { ...prev[section], [key]: value },
    }));
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
      {lightboxImage && (
        <div className="lightbox" onClick={() => setLightboxImage(null)}>
          <img src={lightboxImage} alt="" onClick={e => e.stopPropagation()} />
          <button className="lightbox-close" onClick={() => setLightboxImage(null)}>&#x2715;</button>
        </div>
      )}
      <div className="sidebar">
        <div className="sidebar-header">
          <h2>Shell Agent</h2>
          <button className="new-chat-btn" onClick={handleNewSession}>+ New Chat</button>
          <button className="settings-btn" onClick={openSettings}>Settings</button>
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
                      {editingPinned === i ? (
                        <div className="pinned-edit">
                          <select value={editPinnedCategory} onChange={e => setEditPinnedCategory(e.target.value)}>
                            <option value="fact">fact</option>
                            <option value="preference">preference</option>
                            <option value="decision">decision</option>
                            <option value="context">context</option>
                          </select>
                          <div className="pinned-edit-fields">
                            <input
                              type="text"
                              value={editPinnedNative}
                              onChange={e => setEditPinnedNative(e.target.value)}
                              onCompositionStart={handleCompositionStart}
                              onCompositionEnd={handleCompositionEnd}
                              placeholder="Native"
                              autoFocus
                            />
                            <input
                              type="text"
                              value={editPinnedFact}
                              onChange={e => setEditPinnedFact(e.target.value)}
                              onCompositionStart={handleCompositionStart}
                              onCompositionEnd={handleCompositionEnd}
                              onKeyDown={e => {
                                if (e.key === 'Enter' && !composingRef.current) {
                                  UpdatePinnedMemory(i, editPinnedFact, editPinnedNative, editPinnedCategory).then(() => {
                                    GetPinnedMemories().then(p => setPinnedMemories(p || []));
                                  });
                                  setEditingPinned(null);
                                }
                                if (e.key === 'Escape') setEditingPinned(null);
                              }}
                              placeholder="English"
                            />
                          </div>
                          <button className="pinned-edit-save" onClick={() => {
                            UpdatePinnedMemory(i, editPinnedFact, editPinnedNative, editPinnedCategory).then(() => {
                              GetPinnedMemories().then(p => setPinnedMemories(p || []));
                            });
                            setEditingPinned(null);
                          }}>OK</button>
                        </div>
                      ) : (
                        <>
                          <span className={`pinned-category ${p.category}`}>{p.category}</span>
                          <div className="pinned-content" onDoubleClick={() => { setEditingPinned(i); setEditPinnedFact(p.fact); setEditPinnedNative(p.native_fact || ''); setEditPinnedCategory(p.category); }}>
                            <span className="pinned-fact">{p.native_fact || p.fact}</span>
                            {p.native_fact && p.native_fact !== p.fact && <span className="pinned-fact-en">{p.fact}</span>}
                            <span className="pinned-time">{p.created_at ? new Date(p.created_at).toLocaleString('ja-JP', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : ''}</span>
                          </div>
                          <button className="pinned-delete" onClick={() => setDeletePinnedConfirm(i)}>&#x2715;</button>
                          {deletePinnedConfirm === i && (
                            <div className="pinned-confirm">
                              <button className="pinned-confirm-yes" onClick={() => {
                                DeletePinnedMemory(i).then(() => {
                                  GetPinnedMemories().then(p => setPinnedMemories(p || []));
                                });
                                setDeletePinnedConfirm(null);
                              }}>Delete</button>
                              <button className="pinned-confirm-no" onClick={() => setDeletePinnedConfirm(null)}>Cancel</button>
                            </div>
                          )}
                        </>
                      )}
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
        {showSettings && settings ? (
          <div className="settings-panel">
            <div className="settings-header">
              <h2>Settings</h2>
              <button className="settings-close" onClick={() => setShowSettings(false)}>&#x2715;</button>
            </div>
            <div className="settings-body">
              <div className="settings-section">
                <h3>Appearance</h3>
                <label>
                  <span>Theme</span>
                  <select value={settings.theme || 'dark'} onChange={e => {
                    setSettings((prev: any) => ({ ...prev, theme: e.target.value }));
                    document.documentElement.setAttribute('data-theme', e.target.value);
                  }}>
                    <option value="dark">Dark</option>
                    <option value="light">Light</option>
                    <option value="warm">Warm</option>
                    <option value="midnight">Midnight</option>
                  </select>
                </label>
              </div>

              <div className="settings-section">
                <h3>Startup</h3>
                <label>
                  <span>On Launch</span>
                  <select value={settings.startup_mode || 'new'} onChange={e => {
                    setSettings((prev: any) => ({ ...prev, startup_mode: e.target.value }));
                  }}>
                    <option value="new">New Chat</option>
                    <option value="last">Resume Last Chat</option>
                  </select>
                </label>
              </div>

              <div className="settings-section">
                <h3>API</h3>
                <label>
                  <span>Endpoint</span>
                  <input type="text" value={settings.api?.endpoint || ''} onChange={e => updateSetting('api', 'endpoint', e.target.value)} />
                </label>
                <label>
                  <span>Model</span>
                  <input type="text" value={settings.api?.model || ''} onChange={e => updateSetting('api', 'model', e.target.value)} />
                </label>
                <label>
                  <span>API Key</span>
                  <input type="password" value={settings.api?.api_key || ''} onChange={e => updateSetting('api', 'api_key', e.target.value)} placeholder="(optional)" />
                </label>
              </div>

              <div className="settings-section">
                <h3>Memory</h3>
                <label>
                  <span>Hot Token Limit</span>
                  <input type="number" value={settings.memory?.hot_token_limit || 4096} onChange={e => updateSetting('memory', 'hot_token_limit', parseInt(e.target.value) || 4096)} />
                </label>
                <label>
                  <span>Warm Retention (min)</span>
                  <input type="number" value={settings.memory?.warm_retention_mins || 60} onChange={e => updateSetting('memory', 'warm_retention_mins', parseInt(e.target.value) || 60)} />
                </label>
                <label>
                  <span>Cold Retention (min)</span>
                  <input type="number" value={settings.memory?.cold_retention_mins || 1440} onChange={e => updateSetting('memory', 'cold_retention_mins', parseInt(e.target.value) || 1440)} />
                </label>
              </div>

              <div className="settings-section">
                <h3>Tools</h3>
                <label>
                  <span>Script Directory</span>
                  <input type="text" value={settings.tools?.script_dir || ''} onChange={e => updateSetting('tools', 'script_dir', e.target.value)} />
                </label>
              </div>

              <div className="settings-section">
                <h3>MCP Guardians</h3>
                {(settings.guardians || []).map((g: any, i: number) => (
                  <div key={i} className="guardian-entry">
                    <div className="guardian-header">
                      <span className="guardian-name">{g.name || `Guardian ${i + 1}`}</span>
                      <button className="guardian-remove" onClick={() => {
                        const next = [...(settings.guardians || [])];
                        next.splice(i, 1);
                        setSettings((prev: any) => ({ ...prev, guardians: next }));
                      }}>&#x2715;</button>
                    </div>
                    <label>
                      <span>Name</span>
                      <input type="text" value={g.name || ''} onChange={e => {
                        const next = [...(settings.guardians || [])];
                        next[i] = { ...next[i], name: e.target.value };
                        setSettings((prev: any) => ({ ...prev, guardians: next }));
                      }} />
                    </label>
                    <label>
                      <span>Binary Path</span>
                      <input type="text" value={g.binary_path || ''} onChange={e => {
                        const next = [...(settings.guardians || [])];
                        next[i] = { ...next[i], binary_path: e.target.value };
                        setSettings((prev: any) => ({ ...prev, guardians: next }));
                      }} />
                    </label>
                    <label>
                      <span>Profile Path</span>
                      <input type="text" value={g.profile_path || ''} onChange={e => {
                        const next = [...(settings.guardians || [])];
                        next[i] = { ...next[i], profile_path: e.target.value };
                        setSettings((prev: any) => ({ ...prev, guardians: next }));
                      }} />
                    </label>
                  </div>
                ))}
                <button className="guardian-add" onClick={() => {
                  const next = [...(settings.guardians || []), { name: '', binary_path: 'mcp-guardian', profile_path: '' }];
                  setSettings((prev: any) => ({ ...prev, guardians: next }));
                }}>+ Add Guardian</button>
              </div>
            </div>

            <div className="settings-footer">
              <button className="settings-save" onClick={handleSaveSettings}>Save</button>
              <button className="settings-cancel" onClick={() => setShowSettings(false)}>Cancel</button>
            </div>
          </div>
        ) : (
        <div className="messages">
          {messages.map((msg, i) => (
            <div key={i} className={`message ${msg.role}`}>
              <div className="message-header">
                <span className="message-role">{msg.role}</span>
                <span className="message-time">{msg.timestamp}</span>
              </div>
              {msg.imageIds && msg.imageIds.length > 0 && (
                <div className="message-images">
                  {msg.imageIds.map((id, j) => (
                    <img key={j} src={getCachedImage(id)} alt="" className="message-image" onClick={() => setLightboxImage(getCachedImage(id))} />
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

          {executingTool && (
            <div className="message tool executing">
              <div className="message-content">
                <span className="spinner" />
                <span className="executing-text">Executing: <code>{executingTool}</code></span>
              </div>
            </div>
          )}

          {streaming && !streamContent && !mitlRequest && !executingTool && (
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

        )}
        {!showSettings && (
          <ChatInput onSend={handleSend} disabled={streaming} />
        )}
      </div>
    </div>
  );
}

export default App;
