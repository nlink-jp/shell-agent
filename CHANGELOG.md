# Changelog

All notable changes to this project will be documented in this file.

## [0.6.0] - 2026-04-19

### Added
- Central object repository (`internal/objstore`) for all binary data
- Unique 12-char hex IDs for all objects (images, blobs, reports)
- JSON metadata index with type, MIME, filename, size tracking
- Auto-rebuild index from files on corruption

### Changed
- All image storage migrated from ImageStore to objstore
- All tool artifact (blob) storage migrated from JobManager to objstore
- JobManager simplified: returns Artifact data instead of blob paths
- Image/blob references throughout use objstore IDs
- Object IDs shortened to 12 chars for LLM compatibility
- fileToDataURL checks objstore first by ID
- IMAGE_RECALL_BLOB resolved via objstore
- Storage consolidated: `objects/data/` replaces `images/` and `blobs/`

### Removed
- `internal/memory/ImageStore` (replaced by objstore)
- JobManager blob directory and BlobPath method
- Scattered blob path resolution logic

## [0.5.0] - 2026-04-18

### Added
- Simple agent loop with tool-calling feedback (replaced ReAct)
- Gemma-4 native tool call tag parser (`<|tool_call>call:name{args}<tool_call|>`)
- Tool name fuzzy matching (e.g., `weather:get_current_weather` → `weather`)
- Agent debug logging (`~/Library/.../shell-agent/logs/react.log`)
- Tool execution parameter display inline
- `create-report` built-in tool: markdown reports with image gallery
- Report fullscreen overlay (Expand/Copy/Save)
- Report persistence in session JSON with ImageStore refs
- Image copy to clipboard (macOS osascript) and save to file (native dialog)
- Image actions on hover (Copy/Save) + lightbox toolbar
- Markdown save with inline base64 images (single file, no collision)

### Changed
- Image handling unified: all references use ImageStore IDs
- No data URL round-trips between frontend/backend
- Frontend image cache keyed by ImageStore ID with lazy loading
- System prompt tuned: prefer conversation history over redundant tool calls
- Phase display reset at turn start
- Session records never deleted (removed cleanEphemeralMessages)
- Default hot_token_limit raised to 65536 (auto-correct below 8192)
- Report content in LLM context truncated to 200 chars
- `truncate()` uses rune-based slicing for multibyte safety

### Removed
- ReAct Plan/Execute/Summarize phases (too complex for local LLMs)
- Plan display UI
- Tool list in system prompt (tools via API parameter only)
- cleanEphemeralMessages (session records preservation)

### Fixed
- Gemma tool call leakage in text-only API responses
- Report role mapped to assistant for API compatibility
- Lightbox z-index above report overlay
- MITL dialog width constrained

### Known Issues
- Q4_K_M quantization degrades tool calling accuracy; Q8 recommended
- Multi-step tool workflows depend on model capability
- Image data handling needs central repository (planned for v0.6.0)

## [0.4.0] - 2026-04-18

### Added
- Tool enable/disable toggle per tool in Tools panel
- Disabled tools excluded from LLM and listed in system prompt
- Available tools listed in system prompt for context awareness
- Bulk delete for sessions and pinned memories (Select → All/None → Delete)
- Sidebar collapse/expand with persistent state
- Sidebar resizable by dragging (180-500px)
- Sidebar bottom navigation: Sessions, New Chat, Tools, Status, Settings
- ESC to cancel ongoing LLM requests (HTTP context cancellation)
- Input history (Up/Down arrows, max 50)
- Copy button on message bubbles
- Token usage per message (in/out) and session totals in Status
- Token info persisted in session JSON
- Cmd+Enter to send (Slack-compatible)
- Single newline rendering via remark-breaks
- MCP guardian restart button and hot-reload on settings save
- Configurable max tool rounds (default 10)
- Tool execution images persisted for session restore

### Changed
- ChatInput extracted to memo'd component (major input performance fix)
- Image data URLs stored in ref cache, not React state
- Status polling removed (on-demand only)
- System prompt: proactive tool usage instruction
- Stale tool instruction messages cleaned per turn
- Tool list sorted alphabetically

### Fixed
- Empty `required` array for parameterless tools (API 400 error)
- Sidebar state (width, collapsed) persisted across restarts
- Window shutdown preserves sidebar state
- Session deletion clears chat window
- New sessions saved to disk immediately
- MITL dialog auto-scroll
- Leaked timestamps stripped from LLM responses
- Fake tool call JSON detection and removal

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
