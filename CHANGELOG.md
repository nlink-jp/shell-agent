# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2026-04-18

### Added
- Wails v2 + React + TypeScript project scaffold
- Go backend: chat, client, config, mcp, memory, toolcall packages
- OpenAI-compatible API client (non-streaming for tool calls)
- Multi-turn chat with Markdown rendering (react-markdown, remark-gfm, rehype-highlight)
- Shell script Tool Calling with header-based auto-discovery (@tool, @description, @param, @category)
- MITL (Man-In-The-Loop) approval UI for write/execute operations
- Agent loop with tool execution and result summarization
- Hot/Warm/Cold memory tiers with LLM-powered summarization
- Pinned Memory — autonomous important fact extraction across sessions
- Timestamp injection for temporal awareness in conversations
- Multimodal image support (drag & drop, paste, file picker)
- Smart image recall — only latest image sent as data, past images via view-image tool
- Image lightbox for full-size viewing
- MCP support via mcp-guardian (multiple servers, stdio child processes)
- SwiftUI menu bar launcher with global hotkey (Ctrl+Shift+Space)
- Settings UI (API, memory, tools, MCP guardians)
- Session management (auto-generated titles, rename, delete with confirmation)
- Window position and size persistence
- IME composition guard (ref-based with 50ms delay for WebKit)
- Security: nlk/guard (prompt injection), nlk/jsonfix (JSON repair), nlk/strip (thinking tags)
- Path expansion (~, $ENV) in all config paths
- Custom app icon
- RFP documentation (Japanese and English)
