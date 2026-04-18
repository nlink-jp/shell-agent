# CLAUDE.md — shell-agent

## Overview

macOS local LLM chat & agent tool. Wails v2 (Go + React) main app + SwiftUI launcher.

## Build

- Always `cd app && make build` — never `go build` or `wails build` directly
- Development: `cd app && make dev`
- Launcher: `cd launcher/ShellAgentLauncher && swift build`

## Architecture

- Go backend: `app/app.go` (all bindings + agent loop) + `app/internal/` packages
- React frontend: `app/frontend/src/` — App.tsx (main), ChatInput.tsx (isolated input)
- SwiftUI launcher: `launcher/ShellAgentLauncher/`
- Themes: `app/frontend/src/themes.css` (CSS custom properties)

## Key Design Decisions

- **Non-streaming for tool calls** — streaming unreliable for tool call detection with local LLMs
- **MITL** — required for write/execute tool categories, not for read
- **Job workspace** — each tool execution gets temp dir + blob finalization
- **Memory** — Hot/Warm/Cold with timestamps; Warm via LLM summarization
- **Pinned Memory** — bilingual (English + native), LLM auto-extracts, cross-session
- **Image handling** — latest image as data to LLM; older via view-image tool recall; generated images via blob + tool result event (never embedded in response text)
- **Input performance** — ChatInput is memo'd separate component; image data in ref cache, not state
- **Security** — nlk/guard (nonce per turn), jsonfix (tool args), strip (think tags)
- **MCP** — multiple guardians, one per MCP server, `mcp__guardian__tool` namespace
- **Tool scripts** — `SHELL_AGENT_WORK_DIR` env var; need explicit PATH for non-shell env

## Series

util-series (umbrella: nlink-jp/util-series)
