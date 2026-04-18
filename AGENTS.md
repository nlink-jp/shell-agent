# AGENTS.md — shell-agent

## Project Summary

macOS GUI chat and agent tool powered by local LLM (OpenAI-compatible API).
Wails v2 (Go + React) main app with SwiftUI menu bar launcher.

Key capabilities:
- Shell script Tool Calling with MITL approval and job workspace isolation
- MCP support via mcp-guardian (multiple servers)
- Hot/Warm/Cold sliding window memory with LLM summarization
- Pinned Memory — bilingual autonomous fact extraction across sessions
- Multimodal image support with smart image recall
- nlk security integration (guard, jsonfix, strip)

## Build Commands

```bash
# Main app (Wails v2 + React)
cd app
make build      # Build → dist/shell-agent.app
make dev        # Development mode with hot reload
make test       # Run Go tests

# Launcher (SwiftUI)
cd launcher/ShellAgentLauncher
swift build     # Build → .build/debug/ShellAgentLauncher
```

## Project Structure

```
shell-agent/
├── app/                          # Wails v2 main application
│   ├── main.go                   # Entry point, Wails options, mac titlebar, context menu
│   ├── app.go                    # App struct, Wails bindings, agent loop, all business logic
│   ├── internal/
│   │   ├── chat/                 # Chat engine (message building, time injection)
│   │   ├── client/               # OpenAI-compatible API client (non-streaming + multimodal)
│   │   │                         #   Message, ContentPart, ImageURL, Tool, ToolCall types
│   │   ├── config/               # JSON config, path expansion (~, $ENV), LocationConfig
│   │   ├── mcp/                  # mcp-guardian stdio child process (JSON-RPC 2.0)
│   │   │                         #   Guardian.Start(), Tools(), CallTool()
│   │   ├── memory/               # Hot/Warm/Cold tiers, pinned memory, image store
│   │   │                         #   Session, Record, PinnedStore, ImageStore
│   │   │                         #   EstimateTokens(), PromoteOldestHotToWarm(), ApplySummary()
│   │   └── toolcall/             # Shell script tool registry, header parsing, MITL, jobs
│   │                             #   Registry.Scan(), ExecuteWithJob()
│   │                             #   JobManager, Job (workspace + blob finalization)
│   ├── frontend/
│   │   └── src/
│   │       ├── App.tsx           # Main UI (chat, sidebar, settings, MITL, lightbox)
│   │       ├── App.css           # Theme-variable-based styles
│   │       ├── ChatInput.tsx     # Isolated input component (memo'd for performance)
│   │       ├── themes.css        # CSS custom properties: dark, light, warm, midnight
│   │       └── style.css         # Global styles, scrollbar, theme import
│   ├── build/
│   │   ├── appicon.png
│   │   └── darwin/
│   │       ├── appicon.icns
│   │       └── Info.plist        # NSLocationUsageDescription included
│   ├── wails.json
│   ├── Makefile
│   └── go.mod                    # nlk via local replace directive
├── launcher/                     # SwiftUI menu bar launcher
│   └── ShellAgentLauncher/
│       ├── Package.swift         # macOS 14+
│       └── Sources/
│           ├── ShellAgentLauncherApp.swift   # MenuBarExtra, menu items
│           └── AppDelegate.swift             # Process launch, Ctrl+Shift+Space hotkey
├── docs/
│   ├── en/shell-agent-rfp.md
│   └── ja/shell-agent-rfp.ja.md
├── README.md / README.ja.md
├── CHANGELOG.md
├── CLAUDE.md
└── LICENSE
```

## Architecture Overview

### Agent Loop (app.go SendMessage)

```
User message
  → Clean stale system messages from previous turns
  → Rotate guard tag (nlk/guard) for prompt injection defense
  → Save to Hot memory with timestamp
  → Build messages: system prompt + time/location context + pinned facts
                     + warm/cold summaries + hot messages (guard-wrapped)
  → LLM API call (non-streaming, with tool definitions)
  → If tool calls:
      → For each tool: MITL check → execute in job workspace → finalize blobs
      → Add results to memory, emit to frontend
      → Loop (up to 3 tool rounds, tools removed after success)
  → If text response:
      → Strip think tags (nlk/strip)
      → Strip leaked timestamps
      → Strip image filename references
      → Compact memory if over token limit (LLM summarization)
      → Extract pinned facts (LLM analysis)
      → Generate session title if first exchange
      → Emit to frontend
```

### Memory Architecture

```
┌─────────────────────────────────────────────┐
│ System Prompt                                │
│  + Current time (JST UTC+09:00)              │
│  + Location (if available)                   │
│  + Pinned facts (bilingual, cross-session)   │
├─────────────────────────────────────────────┤
│ Cold summaries (oldest, time-ranged)         │
│ Warm summaries (LLM-generated, time-ranged)  │
├─────────────────────────────────────────────┤
│ Hot messages (full text, timestamped)         │
│  [15:04:05] <guard_tag>user message</guard>  │
│  [15:04:10] assistant response               │
│  [15:04:15] [Tool: name] result              │
└─────────────────────────────────────────────┘

Compaction: Hot tokens > limit → oldest hot → LLM summarize → Warm record
Pinned: LLM extracts category|english|native per turn → persistent JSON
```

### Tool Execution Flow

```
LLM requests tool call
  → jsonfix.Extract(arguments)     # Repair malformed JSON
  → Check builtin tools (list-images, view-image)
  → Check MCP tools (mcp__guardian__tool)
  → Check shell script tools
    → MITL approval if write/execute category
    → JobManager.NewJob() → temp directory
    → Execute script (stdin=JSON, cwd=workdir, env=SHELL_AGENT_*)
    → JobManager.Finalize() → copy artifacts to blobs/job-id/
    → Extract image from result for frontend display
```

### Frontend Architecture

```
App.tsx (main component)
  ├── Sidebar: sessions, tools, status/pinned
  ├── Settings panel (theme, startup, API, memory, tools, guardians)
  ├── Message list (ReactMarkdown + rehype-highlight)
  ├── MITL approval dialog
  ├── Tool execution indicator
  ├── Lightbox overlay
  └── ChatInput.tsx (memo'd, isolated state)
        └── Input + image attach + IME guard
```

- Image data URLs stored in `useRef` cache, NOT in React state
- ChatInput is `memo()`'d — input keystrokes don't re-render message list
- Themes via CSS custom properties on `[data-theme]`

## Module Path

`github.com/nlink-jp/shell-agent`

## Key Dependencies

- **Wails v2** — Go + web frontend desktop framework
- **nlk** — guard (prompt injection), jsonfix (JSON repair), strip (think tags). Local replace
- **mcp-guardian** — MCP governance proxy (stdio child process, installed separately)
- **react-markdown + remark-gfm + rehype-highlight** — Markdown rendering

## Data Locations

| Path | Content |
|------|---------|
| `~/Library/Application Support/shell-agent/config.json` | All settings |
| `~/Library/Application Support/shell-agent/sessions/` | Session JSON files |
| `~/Library/Application Support/shell-agent/images/` | User-attached images |
| `~/Library/Application Support/shell-agent/blobs/` | Tool execution artifacts (job-id/) |
| `~/Library/Application Support/shell-agent/pinned.json` | Cross-session pinned facts |
| `~/Library/Application Support/shell-agent/tools/` | Shell script tools |

## Tool Script Convention

```bash
#!/bin/bash
# @tool: tool-name
# @description: What this tool does
# @param: name type "description"
# @category: read|write|execute
```

- Input: JSON via stdin
- Output: text or JSON via stdout
- Errors: non-zero exit code, stderr captured
- Environment: `SHELL_AGENT_JOB_ID`, `SHELL_AGENT_WORK_DIR`
- Category: `read` (auto-execute), `write`/`execute` (MITL required)

## Gotchas

- `make build` runs `wails build` then copies `.app` bundle to `dist/`
- Frontend must be built before Go embed works (`wails build` handles this)
- mcp-guardian must be installed separately; each instance handles one MCP server
- nlk is referenced via local `replace` directive in go.mod
- IME handling: ref-based composition tracking with 50ms delay on compositionEnd
- Image in LLM context: only most recent image sent as data; older via view-image tool
- Tool scripts need `$HOME/bin` etc. in PATH — add explicitly since Wails doesn't inherit shell env
- Generated images: displayed via tool result event, NOT embedded in LLM response text
- Large base64 data URLs in React state cause severe input lag — use ref cache
