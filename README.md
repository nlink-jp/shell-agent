# shell-agent

A macOS GUI chat and agent tool powered by local LLM.

## Features

- **Multi-turn chat** with OpenAI-compatible API (LM Studio)
- **MCP support** via mcp-guardian (multiple servers, stdio proxy)
- **Shell script Tool Calling** with MITL (Man-In-The-Loop) approval for write/execute operations
- **Timestamp-aware memory** — Hot/Warm/Cold sliding window with LLM summarization
- **Pinned Memory** — autonomous extraction and retention of important facts across sessions
- **Multimodal** — image input via drag & drop, paste, or file picker with smart image recall
- **Markdown rendering** with syntax highlighting (GFM, code blocks, tables)
- **Menu bar launcher** (SwiftUI) with global hotkey (Ctrl+Shift+Space)
- **Security** — nlk/guard (prompt injection defense), nlk/jsonfix (JSON repair), nlk/strip (thinking tag removal)
- **Color themes** — Dark, Light (cream + blue), Warm (brown), Midnight (navy) with live preview
- **Settings UI** — in-app configuration for API, memory, tools, MCP guardians, theme, and startup mode
- **Session management** — auto-generated titles, rename, delete with confirmation
- **Startup mode** — configurable: new chat or resume last session
- **Window state persistence** — position and size remembered across launches

## Architecture

```
shell-agent/
├── app/          # Wails v2 + React main application (Go backend)
├── launcher/     # SwiftUI menu bar launcher (macOS native)
└── docs/         # Documentation and RFP
```

### Go Backend Packages

| Package | Purpose |
|---------|---------|
| `internal/chat` | Chat engine, time injection, message building |
| `internal/client` | OpenAI-compatible API client (streaming + non-streaming, multimodal) |
| `internal/config` | JSON config with path expansion (~, $ENV) |
| `internal/mcp` | mcp-guardian stdio child process management |
| `internal/memory` | Hot/Warm/Cold tiers, pinned memory, image store, session persistence |
| `internal/toolcall` | Shell script tool registry, header parsing, MITL categories |

## Requirements

- macOS 14+ (launcher), macOS 10.15+ (main app)
- [LM Studio](https://lmstudio.ai/) (or any OpenAI-compatible API server)
- Apple Silicon M1/M2 Pro+ recommended (for gemma-4-26b-a4b)

## Build

```bash
# Main app
cd app
make build

# Launcher
cd launcher/ShellAgentLauncher
swift build
```

## Development

```bash
cd app
make dev    # Hot reload with Wails dev server
```

## Tool Scripts

Place shell scripts in `~/Library/Application Support/shell-agent/tools/` with header annotations:

```bash
#!/bin/bash
# @tool: list-files
# @description: List files in a directory
# @param: path string "Directory path to list"
# @category: read
```

Categories: `read` (auto-execute), `write` / `execute` (MITL approval required)

## MCP Configuration

Add MCP servers via Settings UI or `config.json`:

```json
{
  "guardians": [
    {
      "name": "filesystem",
      "binary_path": "~/.local/bin/mcp-guardian",
      "profile_path": "~/.config/mcp-guardian/profiles/filesystem.json"
    }
  ]
}
```

## Default Model

google/gemma-4-26b-a4b

## Configuration

Settings are stored in `~/Library/Application Support/shell-agent/config.json`.
Configurable via the in-app Settings panel.

## License

MIT License - see [LICENSE](LICENSE) for details.
