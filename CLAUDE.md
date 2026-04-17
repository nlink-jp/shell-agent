# CLAUDE.md — shell-agent

## Overview

macOS local LLM chat & agent tool. Wails v2 (Go + React) main app + SwiftUI launcher.

## Build

- Always `cd app && make build` — never `go build` or `wails build` directly
- Development: `cd app && make dev`

## Architecture

- Go backend: `app/internal/` packages (chat, client, config, mcp, memory, toolcall)
- React frontend: `app/frontend/src/`
- SwiftUI launcher: `launcher/` (planned)

## Key Design Decisions

- mcp-guardian handles all MCP communication as stdio child process
- MITL required for write/execute tool categories, not for read
- Memory is Hot/Warm/Cold with timestamps in JSON structure
- nlk packages (guard, jsonfix, strip) for security — must verify API before use

## Series

util-series (umbrella: nlink-jp/util-series)
