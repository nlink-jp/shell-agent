# AGENTS.md — shell-agent

## Project Summary

macOS GUI chat and agent tool powered by local LLM (OpenAI-compatible API).
Features MCP support via mcp-guardian, shell script Tool Calling with MITL,
and timestamp-aware Hot/Warm/Cold sliding window memory.

## Build Commands

```bash
cd app
make build      # Build production binary → dist/
make dev        # Development mode with hot reload
make test       # Run Go tests
```

## Project Structure

```
shell-agent/
├── app/                          # Wails v2 main application
│   ├── main.go                   # Entry point
│   ├── app.go                    # App struct (Wails bindings)
│   ├── internal/
│   │   ├── chat/                 # Chat engine (message building, time injection)
│   │   ├── client/               # OpenAI-compatible API client (streaming)
│   │   ├── config/               # JSON config management
│   │   ├── mcp/                  # mcp-guardian stdio child process
│   │   ├── memory/               # Hot/Warm/Cold session persistence
│   │   └── toolcall/             # Shell script tool registry & execution
│   ├── frontend/                 # React + TypeScript + Vite
│   │   └── src/
│   ├── wails.json
│   ├── Makefile
│   └── go.mod
├── launcher/                     # SwiftUI menu bar launcher (planned)
├── docs/
│   ├── en/shell-agent-rfp.md
│   └── ja/shell-agent-rfp.ja.md
├── README.md
├── README.ja.md
├── CHANGELOG.md
└── LICENSE
```

## Module Path

`github.com/nlink-jp/shell-agent`

## Key Dependencies

- Wails v2 — Go + web frontend desktop framework
- nlk — prompt injection guard, JSON repair, thinking tag strip (to be integrated)
- mcp-guardian — MCP governance proxy (stdio child process)

## Gotchas

- `wails build` outputs to `build/bin/`, Makefile copies to `dist/`
- Frontend must be built before Go embed works (`wails build` handles this)
- mcp-guardian must be installed separately and accessible in PATH or configured path
- Tool scripts require `@tool:` header comment to be recognized
