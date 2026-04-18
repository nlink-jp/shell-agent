# CLAUDE.md — shell-agent

## Overview

macOS local LLM chat & agent tool. Wails v2 (Go + React) main app + SwiftUI launcher.

## Build

- Always `cd app && make build`
- Development: `cd app && make dev`
- Launcher: `cd launcher/ShellAgentLauncher && swift build`

## Architecture

- **app.go** — App struct, all Wails bindings, business logic
- **react.go** — Agent loop, gemma tag parser, debug logging
- **internal/objstore/** — Central object repository (all binary data)
- **internal/memory/** — Hot/Warm/Cold tiers, pinned memory, sessions
- **internal/analysis/** — DuckDB engine, SQL prompts, sliding window summarizer, background CLI
- **internal/toolcall/** — Tool registry, job workspace, artifacts
- **internal/client/** — OpenAI-compatible API client
- **internal/mcp/** — mcp-guardian stdio
- **frontend/src/** — App.tsx (main), ChatInput.tsx (isolated input)

## Key Design Decisions

- **objstore** — Single repository for images/blobs/artifacts. 12-char hex IDs.
- **Agent loop** — Simple tool-calling feedback, not ReAct (too complex for local LLMs)
- **Gemma tag parser** — Fallback for `<|tool_call>` tags in text-only API responses
- **MITL** — Required for write/execute tool categories, not read
- **Memory** — Hot/Warm/Cold + Pinned. Compaction via LLM summarization
- **Image handling** — objstore IDs everywhere. Frontend lazy-loads via GetImageDataURL
- **Reports** — Images as gallery (not inline). Save embeds base64 in markdown
- **No tool list in system prompt** — Tools via API parameter only (prevents leakage)

## Series

util-series (umbrella: nlink-jp/util-series)
