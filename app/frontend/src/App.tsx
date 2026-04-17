import { useState, useEffect, useRef } from 'react';
import './App.css';
import { SendMessage, GetTools, GetLLMStatus, ListSessions, NewSession, LoadSession } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

interface Message {
  role: string;
  content: string;
  timestamp: string;
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

function App() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [streamContent, setStreamContent] = useState('');
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [llmStatus, setLLMStatus] = useState<LLMStatus | null>(null);
  const [sidebarTab, setSidebarTab] = useState<'sessions' | 'tools' | 'status'>('sessions');
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    GetTools().then(setTools);
    ListSessions().then((s) => setSessions(s || []));
    refreshStatus();

    const statusInterval = setInterval(refreshStatus, 5000);

    const offToken = EventsOn('chat:token', (token: string) => {
      setStreamContent(prev => prev + token);
    });

    const offDone = EventsOn('chat:done', () => {
      setStreaming(false);
    });

    return () => {
      clearInterval(statusInterval);
      offToken();
      offDone();
    };
  }, []);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, streamContent]);

  function refreshStatus() {
    GetLLMStatus().then(setLLMStatus);
  }

  async function handleSend() {
    const text = input.trim();
    if (!text || streaming) return;

    const now = new Date();
    const timestamp = now.toLocaleTimeString('ja-JP', { hour12: false });

    const userMsg: Message = { role: 'user', content: text, timestamp };
    setMessages(prev => [...prev, userMsg]);
    setInput('');
    setStreaming(true);
    setStreamContent('');

    try {
      const resp = await SendMessage(text);
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

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  async function handleNewSession() {
    await NewSession();
    setMessages([]);
    ListSessions().then((s) => setSessions(s || []));
    refreshStatus();
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
                <div key={s.id} className="session-item" onClick={() => handleLoadSession(s.id)}>
                  <span className="session-title">{s.title}</span>
                  <span className="session-date">{s.updated_at}</span>
                </div>
              ))}
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
            </div>
          )}
        </div>
      </div>

      <div className="main">
        <div className="messages">
          {messages.map((msg, i) => (
            <div key={i} className={`message ${msg.role}`}>
              <div className="message-header">
                <span className="message-role">{msg.role}</span>
                <span className="message-time">{msg.timestamp}</span>
              </div>
              <div className="message-content">{msg.content}</div>
            </div>
          ))}
          {streaming && streamContent && (
            <div className="message assistant streaming">
              <div className="message-header">
                <span className="message-role">assistant</span>
              </div>
              <div className="message-content">{streamContent}</div>
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>

        <div className="input-area">
          <textarea
            ref={inputRef}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a message... (Enter to send, Shift+Enter for newline)"
            disabled={streaming}
            rows={3}
          />
          <button onClick={handleSend} disabled={streaming || !input.trim()}>
            {streaming ? '...' : 'Send'}
          </button>
        </div>
      </div>
    </div>
  );
}

export default App;
