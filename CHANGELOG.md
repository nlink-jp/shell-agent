# Changelog

All notable changes to this project will be documented in this file.

## [0.7.7] - 2026-04-20

### Changed
- `analyze-bg` replaced with `analyze-data`: in-process synchronous execution instead of detached background process — avoids GPU/CPU contention on local LLM environments
- Dynamic tool filtering: analysis tools only exposed when data is loaded (9 tools without data vs 15 with)
- Removed: JobMonitor, job cards, completion dialog, analysis-status/analysis-result tools (-729 lines)

### Added
- Processing guard: warns before quit or session switch during active analysis/tool execution
- Version in startup log and `GetVersion()` binding

## [0.7.6] - 2026-04-20

### Added
- Sample tool scripts auto-installed on first launch (list-files, write-note, weather, get-location)
- Example tool scripts for gem-search and gem-image (tools/examples/)

## [0.7.5] - 2026-04-20

### Fixed
- RWMutex deadlock in restartGuardians (GetTools called under write lock — caused startup hang)
- Guardian Start() 15-second timeout (prevents indefinite hang on unresponsive MCP)
- HTTP client 10-second connection timeout
- Comprehensive session concurrency (sessionMu on all access sites, snapshot-copy in buildMessages)
- restartGuardians EventsEmit moved outside lock to prevent frontend callback deadlock
- Remove `replace` directive for nlk in go.mod — use published v0.5.1 for portable builds

### Added
- Structured logging: `internal/logger` package (stderr + app.log, component tags, severity levels, 10MB rotation)

## [0.7.4] - 2026-04-19

### Security
- `session.Records` concurrent access: `sessionMu` guards append and range to prevent data race
- `guardians` map rebuild race: `guardiansMu` (RWMutex) protects all read paths during restartGuardians
- HTTP error body truncated to 512 bytes via `io.LimitReader`
- `osascript` clipboard command uses `%q` for path quoting

## [0.7.3] - 2026-04-19

### Security
- Arbitrary file read via `extractImageFromResult`: now delegates to `fileToDataURL` allowlist
- Path traversal via `analysis-status`/`analysis-result` job_id: validated against `job-<digits>` pattern
- API key no longer exposed in `ps` output: passed via `SHELL_AGENT_API_KEY` env var
- `IsReadOnlySQL` rewritten: regex word boundaries, comment/string-literal stripping, multi-statement rejection, DuckDB extension denylists (LOAD/INSTALL/PRAGMA/EXECUTE/VACUUM)
- MCP Guardian stdio race: `Stop()` mutex-guarded and idempotent
- Token-stat data race: `statsMu` + accessor functions

## [0.7.2] - 2026-04-19

### Fixed
- Directory traversal bypass via relative paths: `filepath.Abs()` + separator-aware prefix matching

## [0.7.1] - 2026-04-19

### Fixed
- Directory traversal prevention in `fileToDataURL`: `filepath.Clean()` + allowlist
- SQL read-only enforcement: `IsReadOnlySQL()` at application layer
- Tool script security guidelines added to README

## [0.7.0] - 2026-04-19

### Added
- Data analysis engine with embedded DuckDB (`internal/analysis`)
- Load CSV, JSON, JSONL files into analysis tables (`load-data` tool)
- Direct SQL query execution on loaded data (`query-sql` tool)
- Natural language to SQL generation via LLM (`query-preview` tool)
- Table/column description annotation for LLM context (`describe-data` tool)
- LLM-based analysis perspective suggestion (`suggest-analysis` tool)
- Quick summary of query results via LLM (`quick-summary` tool)
- Sliding window summarization engine for large dataset analysis
- Analysis database reset (`reset-analysis` tool)
- `analyze` subcommand for background analysis mode (single binary, no separate build)
- Prompt injection defense (nlk/guard) in SQL generation and window analysis prompts
- Token estimation with dual word/char-based strategy (CJK aware)
- Finding severity classification and priority-based eviction
- Markdown report generation from analysis results (severity-grouped findings)

### Changed
- Makefile: added `-tags no_duckdb_arrow` (Arrow interface unused, database/sql only)
- Binary size increased (~40MB) due to embedded DuckDB

### Dependencies
- Added `github.com/marcboeker/go-duckdb` v1.8.5 (CGO, embedded DuckDB)

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
- Report generation tool (`create-report`) with Markdown output
- Image gallery display (separate from inline markdown)
- Fullscreen image overlay with Copy and Save actions
- `SaveReport` saves self-contained Markdown with inline base64 images
- Report records in session history (role: "report")
- Image references in reports via `![desc](image:ID)` syntax

## [0.4.0] - 2026-04-18

### Added
- Agent loop with simple tool-calling feedback loop
- Gemma-4 native tag parser (`<|tool_call>call:name{args}<tool_call|>`)
- Fuzzy tool name matching (contains-based fallback)
- Tool execution phase display in chat
- Agent loop debug logging (`logs/react.log`)
- Timestamp leak stripping from LLM responses

## [0.3.0] - 2026-04-18

### Added
- Color themes: Dark, Light (cream + blue), Warm (brown), Midnight (navy)
- CSS custom properties for full theme customization
- Sidebar with collapse/resize and bottom navigation
- Session management: auto-generated titles, rename, delete with confirmation
- Startup mode: new chat or resume last session
- Window state persistence (position, size, sidebar state)
- Token tracking (input/output per message and session total)
- ESC key cancel, input history (Up/Down), copy button

## [0.2.0] - 2026-04-17

### Added
- MCP support via mcp-guardian (multiple servers, stdio JSON-RPC 2.0)
- Multimodal image support (drag & drop, paste, file picker)
- Smart image recall (latest as data URL, older via view-image tool)
- SwiftUI menu bar launcher with Ctrl+Shift+Space hotkey
- nlk security integration (guard, jsonfix, strip)
- Settings UI for API, memory, tools, MCP guardians
- Pinned Memory (bilingual autonomous fact extraction)

## [0.1.0] - 2026-04-17

### Added
- Initial scaffold: Wails v2 + React + Go backend
- Multi-turn chat with OpenAI-compatible API (LM Studio)
- Shell script Tool Calling with MITL approval
- Hot/Warm/Cold memory tiers with LLM summarization
- Markdown rendering with syntax highlighting
- Japanese IME composition handling
