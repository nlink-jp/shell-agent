# RFP: shell-agent

> Generated: 2026-04-18
> Status: Draft

## 1. Problem Statement

A macOS GUI chat and agent tool powered by local LLM. The architecture consists of a SwiftUI menu bar launcher that activates the main application (Wails v2 + React) via keyboard shortcut.

MCP support is provided through mcp-guardian as a stdio proxy. A distinctive feature is the ability to register and execute shell scripts as Tool Calling functions. Write and execute operations require MITL (Man-In-The-Loop) approval to ensure safety.

Multi-turn chat maintains JSON-structured memory with timestamps for each message, managed through a Hot/Warm/Cold three-tier sliding window. This enables the LLM to correctly recognize elapsed time during conversations, properly handling relative time references like "what I said 30 minutes ago."

Multimodal support (image input) enables chat with images.

The backend uses an OpenAI-compatible API (LM Studio), with google/gemma-4-26b-a4b as the default model. No specific target user restriction.

## 2. Functional Specification

### Commands / API Surface

No CLI commands — this is a GUI application.

**Main App (Wails v2 + React):**
- Chat window (message list + input field)
- Image input support (drag & drop, paste, file picker)
- Sidebar: tool list, conversation session list, LLM state monitoring panel (perceived time, memory state, etc.)
- MITL approval UI for Tool Calling execution

**Launcher App (SwiftUI):**
- Menu bar resident
- Keyboard shortcut to launch main app
- New conversation, select existing conversation, settings, quit

### Input / Output

**LLM Communication:**
- OpenAI-compatible `/v1/chat/completions` endpoint (streaming support)
- Request: system prompt + memory context + user message (text + images)
- Images: base64-encoded in content array (OpenAI Vision API format, reusing llm-cli pattern)
- Response: SSE streaming with incremental token display

**Shell Script Tools:**
- Input: JSON via stdin
- Output: text or JSON structure via stdout
- Error: caught by caller, failure reported to LLM

**MCP:**
- mcp-guardian launched as stdio child process
- JSON-RPC 2.0 over stdio

### Configuration

**Settings (JSON file):**
- API endpoint URL
- Default model name
- API key (optional)
- Tool script directory path
- mcp-guardian settings (binary path, config file path)
- Memory settings (Hot token limit, Warm/Cold retention period)
- Keyboard shortcut settings

**Storage location:** `~/Library/Application Support/shell-agent/`

### External Dependencies

| Dependency | Type | Required |
|-----------|------|----------|
| LM Studio (OpenAI-compatible API server) | Local service | Yes |
| mcp-guardian | Binary (stdio child process) | Yes (when using MCP) |
| nlk | Go library (direct import) | Yes |

## 3. Design Decisions

**Language & Framework:**
- Main app: Go + Wails v2 + React — Go backend enables direct use of nlk library and llm-cli API client patterns. Wails v2 is stable with comprehensive documentation. React has the most mature ecosystem and best Wails v2 support.
- Launcher: SwiftUI — optimal for macOS native MenuBarExtra API. Reuses quick-translate implementation patterns.

**Relationship with Existing Tools:**
- `llm-cli` (cli-series): Reuse API client implementation patterns (streaming, retry, format fallback, multimodal image input)
- `nlk` (lib-series): Direct use of guard (prompt injection defense), jsonfix (JSON repair), strip (thinking tag removal), backoff, validate
- `sai` (lab-series): Inherit Hot/Warm/Cold memory tier concept, adapted from RAG to sliding window with timestamp-based temporal awareness
- `mcp-guardian` (util-series): Delegate MCP client functionality; guardian handles auth and governance
- `quick-translate` (util-series): Reuse launcher app MenuBarExtra + AppDelegate + PanelManager pattern

**Out of Scope:**
- Cloud sync
- Multi-user support
- Server mode
- Database (reconsider if scale demands it)

## 4. Development Plan

### Phase 1: Core — Chat Foundation

- Wails v2 + React project scaffold
- OpenAI-compatible API client (reuse llm-cli patterns, streaming support)
- Basic chat UI (message list + input field)
- Hot memory with timestamps (token-based sliding window)
- Conversation persistence (JSON file)
- nlk integration (guard, jsonfix, strip, backoff)
- Basic tests

**Independently reviewable**

### Phase 2: Features — Agent Capabilities

- Shell script Tool Calling
  - Directory scan and header comment parsing for auto-registration
  - JSON I/O via stdin/stdout
  - MITL approval UI (read operations exempt, write/execute operations require approval)
- mcp-guardian integration (stdio child process management)
- Warm/Cold memory tiers (LLM summarization + time range preservation)
- Sidebar (tool list, conversation session list, LLM state monitoring)
- SwiftUI launcher app (menu bar resident, keyboard shortcut)

**Independently reviewable**

### Phase 3: Release — Documentation & Quality

- Test expansion
- README.md / README.ja.md
- CHANGELOG.md
- AGENTS.md
- Release build and distribution

## 5. Required API Scopes / Permissions

None — all external service authentication is delegated to mcp-guardian. Local LLM server requires no authentication (optional API key support).

## 6. Series Placement

Series: **util-series**
Reason: Precedent established by quick-translate for macOS GUI applications in util-series. While not a pipe-friendly CLI, the essence of local data processing and transformation is shared.

## 7. External Platform Constraints

| Constraint | Details |
|-----------|---------|
| LM Studio | Local server must be running. Error handling needed for unavailable state |
| Wails v2 | Requires macOS 10.15+ |
| gemma-4-26b-a4b | Requires ~16GB VRAM (runs on Apple Silicon M1/M2 Pro or above) |

---

## Tool Script Header Format

```bash
#!/bin/bash
# @tool: list-files
# @description: List files in a directory
# @param: path string "Directory path to list"
# @category: read
```

- `@tool`: Tool name (function name exposed to LLM)
- `@description`: Tool description (used by LLM for invocation decisions)
- `@param`: Parameter definition (name type description), multiple lines allowed
- `@category`: `read` (no MITL required) or `write`/`execute` (MITL required)

---

## Memory Structure (JSON)

```json
{
  "timestamp": "2026-04-18T15:30:00+09:00",
  "role": "user",
  "content": "...",
  "tier": "hot",
  "summary_range": null
}
```

Warm/Cold entries:
```json
{
  "timestamp": "2026-04-18T16:00:00+09:00",
  "role": "summary",
  "content": "Summary of 15:00-15:55 conversation: ...",
  "tier": "warm",
  "summary_range": {
    "from": "2026-04-18T15:00:00+09:00",
    "to": "2026-04-18T15:55:00+09:00"
  }
}
```

---

## Discussion Log

1. **Tool name decision**: `shell-agent` — named for its shell script Tool Calling feature
2. **Architecture choice**: Initially considered macOS GUI (SwiftUI), changed to Wails v2 (Go + React) to enable direct use of nlk (Go). Only the launcher remains SwiftUI
3. **MCP approach**: Delegate interface to mcp-guardian, all MCP communication via stdio. No auth management needed in shell-agent
4. **Memory model**: Inherit sai's Hot/Warm/Cold three-tier system, changed from RAG to sliding window. Solve sai's missing temporal awareness with timestamps in JSON structure
5. **MITL policy**: Read functions exempt from MITL; shell execution, write, update, and delete operations require MITL
6. **Security**: nlk/guard for prompt injection defense, nlk/jsonfix for JSON structure repair
7. **Persistence**: JSON files sufficient. Database migration to be reconsidered when scale demands
8. **Frontend**: React selected — most mature and least problematic with Wails v2
9. **Out of scope**: Cloud sync, multi-user, server mode
10. **Series placement**: util-series (following quick-translate precedent)
11. **Multimodal**: Image input support. Reusing llm-cli's existing VLM implementation (base64 encoding, OpenAI Vision API format)
