# AGENTS.md — shell-agent

## Project Summary

macOS GUI chat and agent tool powered by local LLM (OpenAI-compatible API).
Features MCP support via mcp-guardian, shell script Tool Calling with MITL,
timestamp-aware Hot/Warm/Cold sliding window memory with pinned facts,
multimodal image support with smart recall, and nlk security integration.

## Build Commands

```bash
# Main app (Wails v2 + React)
cd app
make build      # Build production binary → build/bin/shell-agent.app
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
│   ├── main.go                   # Entry point, Wails options, mac titlebar
│   ├── app.go                    # App struct, all Wails bindings, agent loop
│   ├── internal/
│   │   ├── chat/                 # Chat engine (message building, time injection)
│   │   ├── client/               # OpenAI-compatible API client (streaming, multimodal)
│   │   ├── config/               # JSON config, path expansion (~, $ENV)
│   │   ├── mcp/                  # mcp-guardian stdio child process (JSON-RPC 2.0)
│   │   ├── memory/               # Hot/Warm/Cold tiers, pinned memory, image store
│   │   └── toolcall/             # Shell script tool registry, header parsing, MITL
│   ├── frontend/                 # React + TypeScript + Vite
│   │   └── src/
│   │       ├── App.tsx           # Main UI component (chat, sidebar, settings, MITL dialog)
│   │       └── App.css           # Dark theme styles
│   ├── build/
│   │   ├── appicon.png           # App icon (PNG for Wails)
│   │   └── darwin/appicon.icns   # App icon (macOS native)
│   ├── wails.json
│   ├── Makefile
│   └── go.mod
├── launcher/                     # SwiftUI menu bar launcher
│   └── ShellAgentLauncher/
│       ├── Package.swift
│       └── Sources/
│           ├── ShellAgentLauncherApp.swift   # MenuBarExtra, menu items
│           └── AppDelegate.swift             # Process launch, global hotkey
├── docs/
│   ├── en/shell-agent-rfp.md
│   └── ja/shell-agent-rfp.ja.md
├── README.md
├── README.ja.md
├── CHANGELOG.md
├── CLAUDE.md
└── LICENSE
```

## Module Path

`github.com/nlink-jp/shell-agent`

## Key Dependencies

- **Wails v2** — Go + web frontend desktop framework
- **nlk** — prompt injection guard, JSON repair, thinking tag strip (local replace)
- **mcp-guardian** — MCP governance proxy (stdio child process, installed separately)
- **react-markdown** — Markdown rendering with GFM and syntax highlighting

## Data Locations

- Config: `~/Library/Application Support/shell-agent/config.json`
- Sessions: `~/Library/Application Support/shell-agent/sessions/`
- Images: `~/Library/Application Support/shell-agent/images/`
- Pinned memories: `~/Library/Application Support/shell-agent/pinned.json`
- Tool scripts: `~/Library/Application Support/shell-agent/tools/`

## Gotchas

- `wails build` outputs to `build/bin/`, Makefile copies to `dist/`
- Frontend must be built before Go embed works (`wails build` handles this)
- mcp-guardian must be installed separately; each instance handles one MCP server
- Tool scripts require `@tool:` header comment to be recognized
- nlk is referenced via local `replace` directive in go.mod (not published)
- IME handling uses ref-based composition tracking with 50ms delay on compositionEnd
- Only the most recent image is sent as actual data to LLM; older images use text + view-image tool
- Launcher searches for main app binary in multiple candidate paths
