# AGENTS.md — shell-agent

## Project Summary

macOS GUI chat and agent tool powered by local LLM (OpenAI-compatible API).
Wails v2 (Go + React) main app with SwiftUI menu bar launcher.

Key capabilities:
- Tool-calling feedback loop with gemma tag parser fallback
- Shell script tools with MITL approval and job workspace
- MCP support via mcp-guardian (multiple servers)
- Hot/Warm/Cold memory with LLM summarization
- Pinned Memory (bilingual autonomous fact extraction)
- Multimodal image support with smart recall
- Report generation with image gallery
- Central object repository for all binary data
- nlk security integration (guard, jsonfix, strip)

## Build Commands

```bash
cd app
make build      # Build → dist/shell-agent.app
make dev        # Development mode with hot reload
make test       # Run Go tests
```

## Project Structure

```
shell-agent/
├── app/
│   ├── main.go                   # Entry point, Wails options
│   ├── app.go                    # App struct, bindings, business logic
│   ├── react.go                  # Agent loop, gemma tag parser, logging
│   ├── internal/
│   │   ├── client/               # OpenAI-compatible API client
│   │   ├── config/               # JSON config, path expansion
│   │   ├── mcp/                  # mcp-guardian stdio (JSON-RPC 2.0)
│   │   ├── memory/               # Hot/Warm/Cold, pinned, session store
│   │   ├── objstore/             # Central object repository ★
│   │   └── toolcall/             # Tool registry, job workspace
│   ├── frontend/src/
│   │   ├── App.tsx / App.css     # Main UI + styles
│   │   ├── ChatInput.tsx         # Isolated input (memo'd)
│   │   └── themes.css            # dark/light/warm/midnight
│   ├── Makefile
│   └── go.mod
├── launcher/ShellAgentLauncher/  # SwiftUI menu bar launcher
└── docs/                         # RFP documentation
```

## Architecture

### Object Store (objstore) — Central Repository

```
~/Library/Application Support/shell-agent/objects/
├── index.json          # {id, type, mime, filename, size, created_at}
└── data/               # Binary files by 12-char hex ID
    ├── a1b2c3d4e5f6.png
    └── f6e5d4c3b2a1.jpg

All binary data flows through objstore:
  User images  → SaveDataURL() → TypeImage
  Tool blobs   → Save()        → TypeBlob
  Report imgs  → SaveDataURL() → TypeImage
  Load         → LoadAsDataURL(id) (mime from index)
```

### Agent Loop (react.go)

```
for round < maxRounds:
  LLM(messages, tools)
  → gemma tag parser fallback if no API tool_calls
  → if text response → done
  → if tool calls:
      execute (MITL if write/execute)
      artifacts → objstore
      results → memory
      continue
```

### Memory Model

```
System Prompt + Time/TZ + Location + Pinned Facts
  ↓
Cold/Warm summaries (time-ranged, LLM-generated)
  ↓
Hot messages (user/assistant/tool/report, timestamped)
  Images: objstore ID references only
```

### Data Locations

| Path | Content |
|------|---------|
| `objects/` | Central binary repository (images, blobs) |
| `sessions/` | Session JSON (references objstore IDs) |
| `pinned.json` | Cross-session remembered facts |
| `config.json` | Settings (API, memory, tools, guardians, theme) |
| `tools/` | Shell script tool definitions |
| `logs/react.log` | Agent debug log |

## Module Path

`github.com/nlink-jp/shell-agent`

## Gotchas

- nlk via local `replace` in go.mod
- Gemma-4 outputs native `<|tool_call>` tags in text-only calls
- Q4_K_M quantization degrades tool calling; Q8 recommended
- 26+ tools may overwhelm local LLM; disable unused via toggle
- Image data in ref cache (not React state) for performance
- Reports: images as gallery, not inline in markdown
- Old ImageStore/blob paths may exist in legacy sessions
