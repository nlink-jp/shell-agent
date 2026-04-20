import { useState, useEffect, useRef, useCallback } from 'react';
import ChatInput from './ChatInput';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkBreaks from 'remark-breaks';
import rehypeHighlight from 'rehype-highlight';
import 'highlight.js/styles/github-dark.css';
import './App.css';
import {
  SendMessage, SendMessageWithImages, GetTools, GetLLMStatus, ListSessions,
  NewSession, LoadSession, DeleteSession, RenameSession,
  ApproveMITL, RejectMITL, GetPinnedMemories,
  UpdatePinnedMemory, DeletePinnedMemory,
  GetConfig, SaveConfig, RestartGuardians, CancelExecution,
  SaveSidebarState, ToggleTool, SaveImageToFile, CopyImageToClipboard,
  SaveReport, GetImageDataURL,
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

interface Message {
  role: string;
  content: string;
  timestamp: string;
  imageIds?: string[];
  in_tokens?: number;
  out_tokens?: number;
  report?: { title: string; filename: string };
}

interface ToolInfo {
  name: string;
  description: string;
  category: string;
  enabled: boolean;
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
  session_input: number;
  session_output: number;
  session_total: number;
  last_input: number;
  last_output: number;
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
  const [executingArgs, setExecutingArgs] = useState<string>('');
  const [currentPhase, setCurrentPhase] = useState<string | null>(null);
  const [pinnedMemories, setPinnedMemories] = useState<PinnedMemory[]>([]);
  const [showSettings, setShowSettings] = useState(false);
  const [settings, setSettings] = useState<any>(null);
  const [lightboxImage, setLightboxImage] = useState<string | null>(null);
  const [expandedReport, setExpandedReport] = useState<{ title: string; filename: string; content: string; imageIds?: string[] } | null>(null);
  // Note: expandedReport.content comes from msg.content, not msg.report.content
  const [sidebarWidth, setSidebarWidth] = useState(280);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const resizingRef = useRef(false);
  const composingRef = useRef(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const imageCacheRef = useRef<Record<string, string>>({});

  // Cache an image by its ImageStore ID. If dataURL is provided, cache it.
  // If only ID is provided, load lazily from backend.
  function cacheImage(idOrDataURL: string): string {
    // If it looks like a data URL, save it and return a temporary ID
    if (idOrDataURL.startsWith('data:')) {
      // This is a data URL from user upload — will be saved by backend
      const tempId = `temp-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
      imageCacheRef.current[tempId] = idOrDataURL;
      return tempId;
    }
    // It's an ImageStore ID — keep as-is
    return idOrDataURL;
  }

  function getCachedImage(id: string): string {
    if (imageCacheRef.current[id]) {
      return imageCacheRef.current[id];
    }
    // Lazy load from backend
    if (id && !id.startsWith('temp-')) {
      GetImageDataURL(id, 'image/png').then(du => {
        if (du) imageCacheRef.current[id] = du;
      });
    }
    return imageCacheRef.current[id] || '';
  }
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Apply theme, restore last session, and fetch location on load
  useEffect(() => {
    GetConfig().then((cfg: any) => {
      if (cfg?.theme) {
        document.documentElement.setAttribute('data-theme', cfg.theme);
      }
      if (cfg?.window?.sidebar_width > 0) {
        setSidebarWidth(cfg.window.sidebar_width);
      }
      if (cfg?.window?.sidebar_collapsed) {
        setSidebarCollapsed(true);
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

    const offToolExecuting = EventsOn('chat:tool_executing', (info: {name: string, args?: string}) => {
      setExecutingTool(info.name);
      setExecutingArgs(info.args || '');
    });

    const offToolResult = EventsOn('chat:toolresult', (res: any) => {
      setExecutingTool(null);
      setExecutingArgs('');
      const now = new Date().toLocaleTimeString('ja-JP', { hour12: false });
      setMessages(prev => [...prev, {
        role: 'tool',
        content: output,
        timestamp: now,
        imageIds: res.imageId ? [res.imageId] : undefined,
      }]);
    });

    const offThinking = EventsOn('chat:thinking', () => {
      setStreamContent('');
    });

    const offPinned = EventsOn('chat:pinned_updated', (entries: PinnedMemory[]) => {
      setPinnedMemories(entries || []);
    });

    const offToolsUpdated = EventsOn('chat:tools_updated', (t: ToolInfo[]) => {
      setTools(t || []);
    });

    const offPhase = EventsOn('chat:phase', (phase: string | null) => {
      setCurrentPhase(phase);
    });

    const offReport = EventsOn('chat:report', (report: { title: string; filename: string; content: string; imageIds?: string[] }) => {
      const now = new Date().toLocaleTimeString('ja-JP', { hour12: false });
      setMessages(prev => [...prev, {
        role: 'report',
        content: report.content,
        timestamp: now,
        report: { title: report.title, filename: report.filename },
        imageIds: report.imageIds,
      }]);
    });

    const offTitleUpdated = EventsOn('chat:title_updated', () => {
      ListSessions().then((s) => setSessions(s || []));
    });

    return () => {
      offToken();
      offDone();
      offToolRequest();
      offToolExecuting();
      offToolResult();
      offThinking();
      offPinned();
      offToolsUpdated();
      offTitleUpdated();
      offPhase();
      offReport();
    };
  }, []);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, streamContent, mitlRequest, executingTool]);

  function refreshStatus() {
    GetLLMStatus().then(setLLMStatus);
  }

  function handleCompositionStart() { composingRef.current = true; }
  function handleCompositionEnd() { setTimeout(() => { composingRef.current = false; }, 50); }

  function startResize(e: React.MouseEvent) {
    e.preventDefault();
    resizingRef.current = true;
    const startX = e.clientX;
    const startWidth = sidebarWidth;
    function onMove(ev: MouseEvent) {
      if (!resizingRef.current) return;
      const newWidth = Math.max(180, Math.min(500, startWidth + ev.clientX - startX));
      setSidebarWidth(newWidth);
    }
    function onUp() {
      resizingRef.current = false;
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      // Save final width
      const el = document.querySelector('.sidebar') as HTMLElement;
      if (el) SaveSidebarState(parseInt(el.style.width) || 280, false);
    }
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  }

  const handleSend = useCallback(async (text: string, images: string[]) => {
    // Ensure current session appears in session list
    ListSessions().then((s) => setSessions(s || []));

    const now = new Date();
    const timestamp = now.toLocaleTimeString('ja-JP', { hour12: false });

    const imageIds = images.length > 0 ? images.map(img => cacheImage(img)) : undefined;
    const userMsg: Message = { role: 'user', content: text || '(image)', timestamp, imageIds };
    setMessages(prev => [...prev, userMsg]);
    setStreaming(true);
    setStreamContent('');
    setCurrentPhase(null);
    setExecutingTool(null);
    setExecutingArgs('');

    try {
      const resp = images.length > 0
        ? await SendMessageWithImages(text, images)
        : await SendMessage(text);
      // Avoid duplicate if the last message already has same content (interim fallback)
      setMessages(prev => {
        const last = prev[prev.length - 1];
        if (last && last.role === 'assistant' && last.content === resp.content) {
          // Update tokens on existing message instead of duplicating
          const updated = [...prev];
          updated[updated.length - 1] = { ...last, in_tokens: resp.in_tokens, out_tokens: resp.out_tokens };
          return updated;
        }
        return [...prev, {
          role: resp.role,
          content: resp.content,
          timestamp: resp.timestamp,
          in_tokens: resp.in_tokens,
          out_tokens: resp.out_tokens,
        }];
      });
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
    if (streaming && !confirm('Processing is in progress. Start a new session? Current results may be lost.')) {
      return;
    }
    const id = await NewSession();
    setMessages([]);
    const now = new Date().toLocaleString('ja-JP', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
    setSessions(prev => [{ id, title: 'New Chat', updated_at: now }, ...prev]);
    refreshStatus();
  }

  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [selectMode, setSelectMode] = useState(false);
  const [selectedSessions, setSelectedSessions] = useState<Set<string>>(new Set());
  const [pinnedSelectMode, setPinnedSelectMode] = useState(false);
  const [selectedPinned, setSelectedPinned] = useState<Set<number>>(new Set());
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

  async function bulkDeleteSessions() {
    for (const id of selectedSessions) {
      try { await DeleteSession(id); } catch {}
    }
    setMessages([]);
    setSelectedSessions(new Set());
    setSelectMode(false);
    ListSessions().then((s) => setSessions(s || []));
    refreshStatus();
  }

  async function bulkDeletePinned() {
    // Delete in reverse order to preserve indices
    const indices = Array.from(selectedPinned).sort((a, b) => b - a);
    for (const i of indices) {
      try { await DeletePinnedMemory(i); } catch {}
    }
    setSelectedPinned(new Set());
    setPinnedSelectMode(false);
    GetPinnedMemories().then(p => setPinnedMemories(p || []));
  }

  function toggleSessionSelect(id: string) {
    setSelectedSessions(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }

  function togglePinnedSelect(i: number) {
    setSelectedPinned(prev => {
      const next = new Set(prev);
      if (next.has(i)) next.delete(i); else next.add(i);
      return next;
    });
  }

  async function confirmDelete() {
    if (!deleteConfirm) return;
    try {
      await DeleteSession(deleteConfirm);
      setMessages([]);
      setStreamContent('');
      ListSessions().then((s) => setSessions(s || []));
      refreshStatus();
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
      imageIds: m.images || undefined, // ImageStore IDs directly
      in_tokens: m.in_tokens,
      out_tokens: m.out_tokens,
      report: m.report || undefined,
    }));
  }

  async function handleLoadSession(id: string) {
    if (streaming && !confirm('Processing is in progress. Switch session? Current results may be lost.')) {
      return;
    }
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
          <div className="lightbox-toolbar" onClick={e => e.stopPropagation()}>
            <button title="Copy" onClick={(e) => {
              const btn = e.currentTarget;
              CopyImageToClipboard(lightboxImage).then(() => {
                btn.textContent = '\u2713 Copied'; setTimeout(() => btn.textContent = 'Copy', 1000);
              });
            }}>Copy</button>
            <button title="Save" onClick={() => {
              SaveImageToFile(lightboxImage);
            }}>Save</button>
            <button onClick={() => setLightboxImage(null)}>Close</button>
          </div>
        </div>
      )}
      {expandedReport && (
        <div className="report-overlay" onClick={() => setExpandedReport(null)}>
          <div className="report-fullscreen" onClick={e => e.stopPropagation()}>
            <div className="report-fullscreen-header">
              <span className="report-title">{expandedReport.title}</span>
              <div className="report-actions">
                <button onClick={() => { navigator.clipboard.writeText(expandedReport.content); }}>Copy</button>
                <button onClick={() => { SaveReport(expandedReport.content || '', expandedReport.filename, expandedReport.imageIds || []).catch((e: any) => alert('Save error: ' + e)); }}>Save</button>
                <button onClick={() => setExpandedReport(null)}>Close</button>
              </div>
            </div>
            {expandedReport.imageIds && expandedReport.imageIds.length > 0 && (
              <div className="report-images" style={{padding: '12px 24px'}}>
                {expandedReport.imageIds.map((id, j) => (
                  <img key={j} src={getCachedImage(id)} alt="" className="report-image" style={{maxHeight: '200px'}} onClick={() => setLightboxImage(getCachedImage(id))} />
                ))}
              </div>
            )}
            <div className="report-fullscreen-content markdown-body">
              <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]} rehypePlugins={[rehypeHighlight]} components={{img: ({src, alt}) => <img src={src} alt={alt || ''} style={{maxWidth: '100%', borderRadius: '8px', margin: '8px 0'}} />}}>
                {expandedReport.content}
              </ReactMarkdown>
            </div>
          </div>
        </div>
      )}
      {sidebarCollapsed && (
        <div className="sidebar-collapsed">
          <div className="sidebar-top">
            <button className="sidebar-nav" onClick={() => { setSidebarCollapsed(false); setSidebarTab('sessions'); }} title="Sessions"><span className="sidebar-nav-ic ic-status">&#x2630;</span></button>
          </div>
          <div className="sidebar-bottom">
            <button className="sidebar-nav" onClick={handleNewSession} title="New Chat"><span className="sidebar-nav-ic">+</span></button>
            <div className="sidebar-bottom-divider" />
            <button className="sidebar-nav" onClick={() => { setSidebarCollapsed(false); setSidebarTab('tools'); }} title="Tools"><span className="sidebar-nav-ic">&#x2692;</span></button>
            <button className="sidebar-nav" onClick={() => { setSidebarCollapsed(false); setSidebarTab('status'); }} title="Status"><span className="sidebar-nav-ic ic-status">&#x2261;</span></button>
            <div className="sidebar-bottom-divider" />
            <button className="sidebar-nav" onClick={openSettings} title="Settings"><span className="sidebar-nav-ic ic-settings">&#x2699;</span></button>
            <div className="sidebar-bottom-divider" />
            <button className="sidebar-nav" onClick={() => { setSidebarCollapsed(false); SaveSidebarState(sidebarWidth, false); }} title="Expand sidebar"><span className="sidebar-nav-ic">&#x25B6;</span></button>
          </div>
        </div>
      )}
      <div className="sidebar" style={{ width: sidebarCollapsed ? 0 : sidebarWidth, display: sidebarCollapsed ? 'none' : undefined }}>
        <div className="sidebar-top">
          <button className={`sidebar-nav ${sidebarTab === 'sessions' ? 'active' : ''}`} onClick={() => setSidebarTab('sessions')}><span className="sidebar-nav-ic ic-status">&#x2630;</span> Sessions</button>
        </div>

        <div className="sidebar-content">
          {sidebarTab === 'sessions' && (
            <div className="session-list">
              {sessions.length > 0 && (
                <div className="bulk-bar">
                  <button className="bulk-toggle" onClick={() => { setSelectMode(!selectMode); setSelectedSessions(new Set()); }}>
                    {selectMode ? 'Cancel' : 'Select'}
                  </button>
                  {selectMode && (
                    <button className="bulk-toggle" onClick={() => {
                      if (selectedSessions.size === sessions.length) setSelectedSessions(new Set());
                      else setSelectedSessions(new Set(sessions.map(s => s.id)));
                    }}>{selectedSessions.size === sessions.length ? 'None' : 'All'}</button>
                  )}
                  {selectMode && selectedSessions.size > 0 && (
                    <button className="bulk-delete" onClick={bulkDeleteSessions}>Delete ({selectedSessions.size})</button>
                  )}
                </div>
              )}
              {sessions.map(s => (
                <div key={s.id} className={`session-item ${selectedSessions.has(s.id) ? 'selected' : ''}`}>
                  {selectMode && (
                    <input type="checkbox" className="session-checkbox" checked={selectedSessions.has(s.id)} onChange={() => toggleSessionSelect(s.id)} />
                  )}
                  <div className="session-info" onClick={() => selectMode ? toggleSessionSelect(s.id) : handleLoadSession(s.id)}>
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
                      <span className="session-title" onDoubleClick={(e) => { if (!selectMode) { e.stopPropagation(); startRename(s.id, s.title); } }}>{s.title}</span>
                    )}
                    <span className="session-date">{s.updated_at}</span>
                  </div>
                  {!selectMode && <button className="session-delete" onClick={(e) => { e.stopPropagation(); handleDeleteSession(s.id); }} title="Delete">&#x2715;</button>}
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
                <div key={t.name} className={`tool-item ${!t.enabled ? 'disabled' : ''}`}>
                  <div className="tool-header">
                    <span className="tool-name">{t.name}</span>
                    <span className={`tool-category ${t.category}`}>{t.category}</span>
                    <label className="tool-toggle">
                      <input type="checkbox" checked={t.enabled} onChange={e => {
                        ToggleTool(t.name, e.target.checked).then(() => GetTools().then(setTools));
                      }} />
                      <span className="tool-toggle-slider" />
                    </label>
                  </div>
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
                <label>Hot Tokens</label>
                <span>{llmStatus.tokens_used?.toLocaleString()} / {llmStatus.token_limit?.toLocaleString()}</span>
              </div>

              <div className="status-section-label">Session Token Usage</div>
              <div className="status-item">
                <label>Last Turn</label>
                <span>In: {llmStatus.last_input?.toLocaleString()} / Out: {llmStatus.last_output?.toLocaleString()}</span>
              </div>
              <div className="status-item">
                <label>Session Total</label>
                <span>{llmStatus.session_total?.toLocaleString()} ({llmStatus.session_input?.toLocaleString()} in + {llmStatus.session_output?.toLocaleString()} out)</span>
              </div>

              <div className="status-item">
                <label>Pinned</label>
                <span>{pinnedMemories.length} facts</span>
              </div>
              {pinnedMemories.length > 0 && (
                <div className="pinned-list">
                  <div className="bulk-bar">
                    <label className="pinned-header">Remembered Facts</label>
                    <button className="bulk-toggle" onClick={() => { setPinnedSelectMode(!pinnedSelectMode); setSelectedPinned(new Set()); }}>
                      {pinnedSelectMode ? 'Cancel' : 'Select'}
                    </button>
                    {pinnedSelectMode && (
                      <button className="bulk-toggle" onClick={() => {
                        if (selectedPinned.size === pinnedMemories.length) setSelectedPinned(new Set());
                        else setSelectedPinned(new Set(pinnedMemories.map((_, i) => i)));
                      }}>{selectedPinned.size === pinnedMemories.length ? 'None' : 'All'}</button>
                    )}
                    {pinnedSelectMode && selectedPinned.size > 0 && (
                      <button className="bulk-delete" onClick={bulkDeletePinned}>Delete ({selectedPinned.size})</button>
                    )}
                  </div>
                  {pinnedMemories.map((p, i) => (
                    <div key={i} className={`pinned-item ${selectedPinned.has(i) ? 'selected' : ''}`}>
                      {pinnedSelectMode && (
                        <input type="checkbox" className="pinned-checkbox" checked={selectedPinned.has(i)} onChange={() => togglePinnedSelect(i)} />
                      )}
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
                          {!pinnedSelectMode && <button className="pinned-delete" onClick={() => setDeletePinnedConfirm(i)}>&#x2715;</button>}
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
        <div className="sidebar-bottom">
          <button className="sidebar-nav" onClick={handleNewSession}><span className="sidebar-nav-ic">+</span> New Chat</button>
          <div className="sidebar-bottom-divider" />
          <button className={`sidebar-nav ${sidebarTab === 'tools' ? 'active' : ''}`} onClick={() => setSidebarTab(sidebarTab === 'tools' ? 'sessions' : 'tools')}><span className="sidebar-nav-ic">&#x2692;</span> Tools</button>
          <button className={`sidebar-nav ${sidebarTab === 'status' ? 'active' : ''}`} onClick={() => setSidebarTab(sidebarTab === 'status' ? 'sessions' : 'status')}><span className="sidebar-nav-ic ic-status">&#x2261;</span> Status</button>
          <div className="sidebar-bottom-divider" />
          <button className="sidebar-nav" onClick={openSettings}><span className="sidebar-nav-ic ic-settings">&#x2699;</span> Settings</button>
          <div className="sidebar-bottom-divider" />
          <button className="sidebar-nav" onClick={() => { setSidebarCollapsed(true); SaveSidebarState(sidebarWidth, true); }} title="Collapse sidebar"><span className="sidebar-nav-ic">&#x25C0;</span></button>
        </div>
      </div>

      {!sidebarCollapsed && <div className="sidebar-resize" onMouseDown={startResize} />}
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
                <label>
                  <span>Max Tool Rounds</span>
                  <input type="number" value={settings.memory?.max_tool_rounds || 10} onChange={e => updateSetting('memory', 'max_tool_rounds', parseInt(e.target.value) || 10)} />
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
                <div className="guardian-buttons">
                  <button className="guardian-add" onClick={() => {
                    const next = [...(settings.guardians || []), { name: '', binary_path: 'mcp-guardian', profile_path: '' }];
                    setSettings((prev: any) => ({ ...prev, guardians: next }));
                  }}>+ Add Guardian</button>
                  <button className="guardian-restart" onClick={() => {
                    RestartGuardians().then((count) => {
                      GetTools().then(setTools);
                    });
                  }}>Restart All</button>
                </div>
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
                    <div key={j} className="message-image-wrap">
                      <img src={getCachedImage(id)} alt="" className="message-image" onClick={() => setLightboxImage(getCachedImage(id))} />
                      <div className="image-actions">
                        <button title="Copy image" onClick={(e) => {
                          const btn = e.currentTarget;
                          CopyImageToClipboard(getCachedImage(id)).then(() => {
                            btn.textContent = '\u2713'; setTimeout(() => btn.textContent = 'Copy', 1000);
                          });
                        }}>Copy</button>
                        <button title="Save image" onClick={() => {
                          SaveImageToFile(getCachedImage(id));
                        }}>Save</button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
              {msg.report ? (
                <div className="report-container">
                  <div className="report-header">
                    <span className="report-title">{msg.report.title}</span>
                    <div className="report-actions">
                      <button onClick={() => setExpandedReport({ title: msg.report!.title, filename: msg.report!.filename, content: msg.content, imageIds: msg.imageIds })}>Expand</button>
                      <button onClick={() => { navigator.clipboard.writeText(msg.content); }}>Copy</button>
                      <button onClick={() => { SaveReport(msg.content, msg.report!.filename, msg.imageIds || []); }}>Save</button>
                    </div>
                  </div>
                  {msg.imageIds && msg.imageIds.length > 0 && (
                    <div className="report-images">
                      {msg.imageIds.map((id, j) => (
                        <img key={j} src={getCachedImage(id)} alt="" className="report-image" onClick={() => setLightboxImage(getCachedImage(id))} />
                      ))}
                    </div>
                  )}
                  <div className="report-content markdown-body" onClick={() => setExpandedReport({ title: msg.report!.title, filename: msg.report!.filename, content: msg.content, imageIds: msg.imageIds })}>
                    <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]} rehypePlugins={[rehypeHighlight]} components={{img: ({src, alt}) => <img src={src} alt={alt || ''} style={{maxWidth: '100%', borderRadius: '8px', margin: '8px 0'}} />}}>
                      {msg.content}
                    </ReactMarkdown>
                  </div>
                </div>
              ) : (
                <div className="message-content markdown-body">
                  <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]} rehypePlugins={[rehypeHighlight]} components={{img: ({src, alt}) => <img src={src} alt={alt || ''} style={{maxWidth: '100%', borderRadius: '8px', margin: '8px 0'}} />}}>
                    {msg.content}
                  </ReactMarkdown>
                </div>
              )}
              <div className="message-footer">
                <div className="message-footer-left">
                  <button className="message-copy" onClick={(e) => { navigator.clipboard.writeText(msg.content); const b = e.currentTarget; b.classList.add('copied'); setTimeout(() => b.classList.remove('copied'), 1000); }} title="Copy"><span className="copy-icon">{'\u2398'}</span><span className="copy-check">{'\u2713'}</span></button>
                </div>
                {msg.in_tokens != null && (
                  <span className="message-tokens">
                    in:{msg.in_tokens.toLocaleString()} / out:{msg.out_tokens?.toLocaleString()}
                  </span>
                )}
              </div>
            </div>
          ))}

          {executingTool && (
            <div className="message tool executing">
              <div className="message-content">
                <span className="spinner" />
                <span className="executing-text">Executing: <code>{executingTool}</code></span>
                {executingArgs && executingArgs !== '{}' && (
                  <pre className="executing-args">{(() => { try { return JSON.stringify(JSON.parse(executingArgs), null, 2); } catch { return executingArgs; } })()}</pre>
                )}
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
                <span className="thinking-text">
                  {currentPhase === 'plan' ? 'Planning...' :
                   currentPhase === 'summarize' ? 'Summarizing...' :
                   currentPhase?.startsWith('execute') ? currentPhase.replace('execute', 'Executing') + '...' :
                   'Thinking...'}
                </span>
              </div>
            </div>
          )}


          {streaming && streamContent && (
            <div className="message assistant streaming">
              <div className="message-header">
                <span className="message-role">assistant</span>
              </div>
              <div className="message-content markdown-body">
                <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]} rehypePlugins={[rehypeHighlight]} components={{img: ({src, alt}) => <img src={src} alt={alt || ''} style={{maxWidth: '100%', borderRadius: '8px', margin: '8px 0'}} />}}>
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
          <ChatInput onSend={handleSend} onCancel={() => { CancelExecution(); setStreaming(false); }} disabled={streaming} />
        )}
      </div>
    </div>
  );
}

export default App;
