# Changelog

All notable changes to this project will be documented in this file.

## [0.3.0] - 2026-04-18

### Added
- Job workspace system: each tool execution gets a unique job ID, temp directory, and persistent blob storage
- SHELL_AGENT_WORK_DIR environment variable passed to tool scripts
- Bilingual pinned memory: English + native language expressions
- Pinned memory edit (double-click) and delete (with confirmation) in Status tab
- Pinned memory timestamps displayed
- Tool execution indicator: spinning "Executing: tool-name" display
- Tool execution timeout (3 minutes) to prevent hangs
- Tool retry: up to 3 rounds of tool calls per turn
- Location tool script (get-location): timezone-based inference, no external API
- Weather tool script (weather): JMA XML feed with region alias mapping
- Web search tool script (web-search): wraps gem-search CLI
- Image generation tool script (generate-image): wraps gem-image CLI with job workspace
- Blob artifact images displayed in tool results
- view-image tool searches blob storage directories
- Right-click context menu enabled for DevTools access

### Changed
- ChatInput extracted to separate React component (memo'd) for input performance
- Image data URLs stored in ref cache instead of React state
- Status polling removed (update on message send only)
- Stale tool instruction messages cleaned at start of each turn
- System messages hidden from session restore UI
- Tool results stored as "tool" role in memory (converted to "user" for LLM API)
- MITL buttons use opaque backgrounds with white text for all-theme readability
- LLM response text: bare image filenames and markdown image refs stripped
- Explicit timezone in system prompt (JST UTC+09:00)
- Leaked [HH:MM:SS] timestamps stripped from LLM responses

### Fixed
- Empty `required` array (was null) for parameterless tools causing API 400 error
- Empty LLM responses no longer create blank message bubbles
- Success/failure differentiated system messages prevent unnecessary tool re-calls

## [0.2.0] - 2026-04-18

### Added
- Color theme switching: Dark, Light (cream + blue), Warm (brown), Midnight (navy)
- Theme selector in Settings → Appearance with live preview
- Configurable startup mode: "New Chat" or "Resume Last Chat"
- Last session ID auto-saved on shutdown for resume

### Changed
- All CSS colors migrated to CSS custom properties for theming
- Guardian "Add" button uses theme-aware colors instead of hardcoded purple

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
