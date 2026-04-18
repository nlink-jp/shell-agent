# CLAUDE.md — shell-agent

## Overview

macOS local LLM chat & agent tool. Wails v2 (Go + React) main app + SwiftUI launcher.

## Build

- Always `cd app && make build` — never `go build` or `wails build` directly
- Development: `cd app && make dev`
- Launcher: `cd launcher/ShellAgentLauncher && swift build`

## Architecture

- Go backend: `app/internal/` packages (chat, client, config, mcp, memory, toolcall)
- React frontend: `app/frontend/src/` (App.tsx is the single main component)
- SwiftUI launcher: `launcher/ShellAgentLauncher/`

## Key Design Decisions

- mcp-guardian handles all MCP communication as stdio child process (one per server)
- MITL required for write/execute tool categories, not for read
- Memory is Hot/Warm/Cold with timestamps in JSON structure
- Pinned Memory: LLM autonomously extracts important facts, persists across sessions
- Only latest image sent as actual data to LLM; older images via view-image tool
- nlk packages (guard, jsonfix, strip) for security — local replace in go.mod
- IME guard uses ref + 50ms delay on compositionEnd (WebKit race condition)

## Series

util-series (umbrella: nlink-jp/util-series)
