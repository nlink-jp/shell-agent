# Shell Agent — System Architecture

> Status: v0.7.0
> Date: 2026-04-19

## Design Philosophy

shell-agent is a macOS GUI chat and agent tool built for **local LLM inference**. Every architectural decision optimizes for the constraints and opportunities of running a large language model on the user's own hardware:

- **No cloud dependency** — All intelligence runs locally via OpenAI-compatible API (LM Studio)
- **Memory efficiency** — Three-tier sliding window prevents context overflow without losing history
- **Tool reliability** — Simple feedback loop instead of ReAct; gemma tag parser as fallback for models that embed tool calls in text
- **User control** — MITL approval for dangerous operations; tool enable/disable toggles
- **Data sovereignty** — Everything stored locally: sessions, objects, pinned memories, analysis databases

## Technology Stack

```
┌──────────────────────────────────────────────┐
│  SwiftUI Launcher (menu bar, hotkey)         │
└──────────────┬───────────────────────────────┘
               │ launch
┌──────────────▼───────────────────────────────┐
│  Wails v2 Application                        │
│  ┌─────────────────┐  ┌───────────────────┐  │
│  │  Go Backend      │  │  React Frontend   │  │
│  │  ┌─────────────┐ │  │  App.tsx          │  │
│  │  │ app.go      │ │  │  ChatInput.tsx    │  │
│  │  │ react.go    │ │  │  themes.css       │  │
│  │  │ internal/   │ │  │                   │  │
│  │  └─────────────┘ │  └───────────────────┘  │
│  └─────────────────┘                          │
└──────────────────────────────────────────────┘
         │              │              │
    ┌────▼────┐   ┌────▼────┐   ┌────▼────┐
    │ LM      │   │ mcp-    │   │ Shell   │
    │ Studio  │   │ guardian │   │ Scripts │
    │ (API)   │   │ (stdio) │   │ (tools) │
    └─────────┘   └─────────┘   └─────────┘
```

## Component Map

### Go Backend Packages

| Package | Responsibility | Key Innovation |
|---------|---------------|----------------|
| `app.go` | Wails bindings, tool routing, business logic | Unified tool dispatch (shell + MCP + builtin) |
| `react.go` | Agent loop, gemma tag parser, debug logging | Gemma-4 native tag fallback |
| `internal/memory` | Hot/Warm/Cold tiers, pinned memory, sessions | Time-aware sliding window with LLM compaction |
| `internal/client` | OpenAI-compatible API (streaming + non-streaming) | Multimodal support with labeled images |
| `internal/toolcall` | Shell script registry, job workspace, MITL | Header-based auto-discovery |
| `internal/mcp` | mcp-guardian stdio, JSON-RPC 2.0 | Multi-server namespace isolation |
| `internal/objstore` | Central binary repository | 12-char hex IDs, auto-rebuild index |
| `internal/analysis` | DuckDB engine, SQL generation, summarizer | Two-layer analysis (interactive + background) |
| `internal/config` | JSON config, path expansion | ~/\$ENV expansion in all paths |

### Frontend (React + TypeScript)

| File | Responsibility |
|------|---------------|
| `App.tsx` | Main UI: chat, sidebar, reports, lightbox, settings |
| `ChatInput.tsx` | Isolated input (memo'd): IME guard, Cmd+Enter, history |
| `themes.css` | CSS custom properties: dark/light/warm/midnight |

## Data Flow: A Complete Turn

```
1. User types message + optional images
   → ChatInput.tsx (IME composition guard, 50ms WebKit delay)
   → App.SendMessageWithImages(content, images[])

2. Images saved to objstore
   → Each data URL → objstore.SaveDataURL() → 12-char ID
   → Record created with ImageEntry{ID} references

3. System prompt assembled
   → Time + timezone + location
   → Pinned memories (English)
   → Warm/cold summaries (chronological)
   → Guard tag wrapping (nonce rotation per turn)

4. Agent loop begins (react.go)
   → buildMessages() from session records
   → Smart image handling: latest as data URL, older as text refs
   → LLM call with tool definitions

5. Response processing
   a. Tool calls returned → execute sequentially
      → MITL check for write/execute categories
      → Job workspace created per tool
      → Artifacts → objstore
      → Results → memory records
      → Loop continues (up to maxRounds)

   b. Text response → strip leaked timestamps
      → Save to memory
      → Return to frontend

6. Post-turn maintenance
   → Token stats updated
   → Memory compaction if over budget
   → Pinned memory extraction (bilingual)
   → Session auto-saved
```

## Security Architecture

### Defense in Depth

| Layer | Mechanism | Purpose |
|-------|-----------|---------|
| Prompt injection | nlk/guard nonce-tagged XML | Prevent data from being treated as instructions |
| JSON repair | nlk/jsonfix | Handle malformed LLM output |
| Thinking tags | nlk/strip | Remove model thinking leakage |
| Tool approval | MITL (Man-In-The-Loop) | User confirms dangerous operations |
| SQL restriction | Prompt + DryRun | SELECT only, validated before execution |
| Image isolation | objstore IDs (not data URLs) | No base64 in memory/state |

### Guard Tag Flow

```go
// Per-turn rotation
guardTag = guard.NewTag()                    // Random nonce
sys = guardTag.Expand("...{{DATA_TAG}}...")   // Instructions reference nonce
input = guardTag.Wrap(userText)              // Data wrapped in nonce tags
```

The LLM sees instructions to treat `<nonce>` content as data only. Even if a user's input contains "ignore all previous instructions", it's inside tagged data that the model is instructed to treat opaquely.

## Storage Layout

```
~/Library/Application Support/shell-agent/
├── config.json              # API, memory, tools, MCP, theme, window state
├── pinned.json              # Cross-session bilingual memories
├── sessions/                # Conversation JSON files
│   └── session-{ms}.json   # Full record history per session
├── objects/                 # Central binary repository
│   ├── index.json          # Metadata: id, type, mime, size
│   └── data/               # Binary files: {id}.{ext}
├── analysis/               # Data analysis subsystem
│   ├── analysis.duckdb     # Persistent analysis database
│   └── job-{ms}/           # Background job directories
├── tools/                  # User shell script tools
└── logs/
    └── react.log           # Agent loop debug log
```

## Performance Considerations

### Input Performance
- ChatInput is a separate `React.memo()` component — prevents full re-render on keystroke
- Image data URLs stored in `useRef` cache, not React state — avoids reconciliation
- IME composition tracked via ref with 50ms delay for WebKit race condition

### Memory Performance
- Images referenced by 12-char ID, never stored as data URLs in memory
- Frontend lazy-loads images via `GetImageDataURL(id)` on demand
- Session JSON contains IDs only — a session with 50 images is still small

### LLM Context Efficiency
- Only latest image sent as full data URL to VLM
- Older images referenced as text: `[Past image ID: {id}]`
- Tool results in hot memory, summaries in warm — natural token decay
- Report content truncated to 200 chars in LLM context

## Related Architecture Documents

- [Memory Architecture](memory-architecture.md) — Hot/Warm/Cold tiers, pinned memory, time awareness
- [Agent & Tool Architecture](agent-tool-architecture.md) — Agent loop, gemma parser, MCP, MITL
- [Analysis Architecture](analysis-architecture.md) — DuckDB, sliding window, background analysis
