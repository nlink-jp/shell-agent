# shell-agent

A macOS GUI chat and agent tool powered by local LLM.

## Features

- Multi-turn chat with OpenAI-compatible API (LM Studio)
- MCP support via mcp-guardian (stdio proxy)
- Shell script Tool Calling with MITL (Man-In-The-Loop) approval
- Timestamp-aware Hot/Warm/Cold sliding window memory
- Multimodal image input
- Menu bar launcher with keyboard shortcut

## Architecture

```
shell-agent/
├── app/          # Wails v2 + React main application (Go backend)
├── launcher/     # SwiftUI menu bar launcher (macOS native)
└── docs/         # Documentation and RFP
```

## Requirements

- macOS 10.15+
- LM Studio (or any OpenAI-compatible API server)
- Apple Silicon M1/M2 Pro+ recommended (for gemma-4-26b-a4b)

## Build

```bash
cd app
make build
```

## Development

```bash
cd app
make dev
```

## Default Model

google/gemma-4-26b-a4b

## Configuration

Settings are stored in `~/Library/Application Support/shell-agent/config.json`.

## License

MIT License - see [LICENSE](LICENSE) for details.
