# Agent & Tool Architecture — shell-agent

> Status: v0.7.0
> Date: 2026-04-19

## Overview

shell-agent's agent loop orchestrates three tool execution channels — shell scripts, MCP servers, and built-in tools — through a unified feedback loop. The architecture prioritizes reliability with local LLMs by keeping the loop simple and providing fallback parsing for models that embed tool calls in text.

## Design Decisions

### Why Simple Feedback Loop (not ReAct)

**Considered alternative:** ReAct (Reason + Act) pattern with explicit Plan/Execute/Summarize phases.

**What happened:** ReAct was implemented and tested with gemma-4-26b-a4b. Results:
- Interim summaries triggered the model to output tool call tags in text responses
- Plan quality was inconsistent — the model would generate plans it couldn't follow
- Three distinct LLM calls per iteration consumed too many tokens
- Total complexity exceeded what a quantized local model could handle reliably

**Chosen approach:** Simple tool-calling feedback loop.

```
for round < maxRounds:
    LLM(messages, tools) →
    │
    ├─ text response → done (return to user)
    │
    └─ tool calls → execute each → append results → continue loop
```

**Rationale:** The LLM decides when to call tools and when to respond. No explicit planning phase. The model naturally converges after 1-3 tool calls. Max rounds (default 10) prevents infinite loops.

### Why Gemma Tag Parser

gemma-4 uses native tool call tags (`<|tool_call>call:name{args}<tool_call|>`) when the API doesn't return structured tool calls. This happens when:
- The API server (LM Studio) returns text-only responses for certain prompts
- The model outputs tool calls in text even when tools parameter is provided

Without the parser, these would appear as broken text in the chat.

## Agent Loop (`react.go`)

### Core Function

```go
func (a *App) agentLoop(ctx context.Context, systemPrompt string, toolDefs []client.Tool) (ChatMessage, error)
```

### Lifecycle per Turn

```
1. Initialize: maxRounds from config, create agentLog

2. Loop (round 0..maxRounds-1):
   a. Build messages from session records (buildMessages)
   b. Call LLM: ChatWithContext(ctx, messages, toolDefs)
   c. Track tokens: resp.Usage → tokenStats

   d. Parse response:
      ├─ API tool_calls present? → use directly
      ├─ No API tool_calls, text contains gemma tags?
      │   → parseGemmaToolCalls(text) → synthetic tool calls
      └─ No tool calls at all?
          ├─ Empty text? → continue (retry)
          └─ Has text? → stripLeakedTimestamps → return

   e. For each tool call:
      ├─ jsonfix.Extract(arguments) → repair JSON
      ├─ handleBuiltinTool(name, args) → try built-in first
      ├─ handleMCPTool(name, args) → try MCP namespace
      ├─ handleShellTool(name, args) → shell script registry
      └─ fuzzyMatch(name, registry) → fallback: contains-based match
      
      → Save result as Record{Role: "tool", Tier: hot}
      → Extract images from result → objstore

   f. Continue loop

3. Loop exhausted → return last response or error
```

### Gemma Tag Parser

```go
func parseGemmaToolCalls(text string) []client.ToolCall
```

**Supported formats:**
```
<|tool_call>call:weather{"location":"Tokyo"}<tool_call|>
<tool_call>call:search{"query":"DuckDB"}</tool_call>
```

**Parsing steps:**
1. Find tag boundaries (`<|tool_call>` or `<tool_call>`)
2. Extract inner text after `call:`
3. Split at first `{` → tool name + JSON args
4. Repair JSON via `nlk/jsonfix.Extract()`
5. Assign synthetic IDs: `"gemma-0"`, `"gemma-1"`, ...

**Fuzzy tool name matching:**
```go
// LLM sometimes outputs "weather:get_current_weather" instead of "weather"
for _, t := range registry {
    if strings.Contains(llmName, t.Name) {
        // Match: "weather:get_current_weather" contains "weather"
        return t
    }
}
```

### Debug Logging

```go
type agentLog struct {
    file *os.File
}
```

- Location: `{ConfigDir}/logs/react.log`
- Appended per turn with timestamp header
- Logs: round number, message count, tool call details, execution results, errors
- Buffered file I/O for performance

## Tool Execution Channels

### Channel 1: Built-in Tools

Handled directly in `app.go` via `handleBuiltinTool()`:

| Tool | Purpose | LLM Integration |
|------|---------|-----------------|
| `list-images` | List conversation images with timestamps | Image recall workflow |
| `view-image` | Recall a specific past image by ID | Returns `__IMAGE_RECALL__` marker |
| `create-report` | Generate markdown report with images | Report overlay in UI |
| `load-data` | Load CSV/JSON/JSONL into DuckDB | Analysis pipeline |
| `describe-data` | Show/annotate table schemas | Schema context for SQL |
| `query-preview` | Natural language → SQL → results | LLM generates SQL |
| `query-sql` | Direct SQL execution | Precise control |
| `suggest-analysis` | LLM suggests analysis perspectives | Meta-analysis |
| `quick-summary` | Query + LLM summarization | Insight extraction |
| `analyze-bg` | Spawn background analysis | Detached process |
| `analysis-status` | Check background job progress | Status polling |
| `analysis-result` | Retrieve completed report | Report delivery |
| `reset-analysis` | Clear analysis database | Fresh start |

### Channel 2: MCP Tools (mcp-guardian)

**Architecture:**
```
shell-agent ←stdio→ mcp-guardian ←stdio→ MCP server
                     (auth proxy)
```

**Guardian lifecycle:**
```go
type Guardian struct {
    cmd    *exec.Cmd       // Child process
    stdin  io.WriteCloser  // JSON-RPC requests
    stdout *bufio.Scanner  // JSON-RPC responses (1MB buffer)
    mu     sync.Mutex      // Thread safety
    id     int             // Request counter
    tools  []ToolDef       // Discovered tools
}
```

1. `Start()` → spawn binary, pipe stdio, initialize, discover tools
2. `CallTool(name, args)` → JSON-RPC 2.0 request/response
3. `Stop()` → close stdin, kill process

**JSON-RPC 2.0 Protocol:**
```json
→ {"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"q":"test"}}}
← {"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"..."}]}}
```

**Multi-server namespace:**
- Tools prefixed: `mcp__<guardianName>__<toolName>`
- Example: `mcp__filesystem__read_file`
- Each guardian is an independent child process
- Guardians hot-reloaded on settings save

### Channel 3: Shell Script Tools

**Discovery:**
```bash
#!/bin/bash
# @tool: get-weather
# @description: Get current weather for a location
# @param: location string "City name or coordinates"
# @category: read
```

- Scripts placed in `~/Library/Application Support/shell-agent/tools/`
- `Registry.Scan()` reads first 50 lines of each file
- Header comments parsed into `ToolScript` struct
- Re-scanned on app restart

**Execution:**
```go
func (r *Registry) ExecuteWithJob(name, argsJSON string, jobMgr *JobManager) (*ExecuteResult, error)
```

1. Create job workspace: temp directory per execution
2. Set environment variables:
   - `SHELL_AGENT_JOB_ID` — unique job identifier
   - `SHELL_AGENT_WORK_DIR` — temp directory path
3. Pass arguments as JSON via stdin
4. Execute with 3-minute timeout
5. Collect stdout as result text
6. Finalize: scan workspace for produced files → Artifacts
7. Cleanup temp directory

**Artifact collection:**
```go
type Artifact struct {
    Name     string // Filename
    MimeType string // Detected from extension
    Data     []byte // File contents
}
```

All artifacts are saved to objstore and their IDs appended to the tool output:
```
Tool output text here
[Artifacts produced: a1b2c3d4e5f6 f6e5d4c3b2a1]
```

## MITL (Man-In-The-Loop)

### Category System

| Category | MITL Required | Examples |
|----------|--------------|---------|
| `read` | No | file listing, API queries, data viewing |
| `write` | Yes | file creation, data modification |
| `execute` | Yes | shell commands, process spawning |

Built-in tools bypass MITL (they're trusted internal code).

### Approval Flow

```
1. Tool call received from LLM
2. Category checked: tool.Category.NeedsMITL()
3. If MITL required:
   → Emit "chat:toolcall_request" event to frontend
   → Frontend shows approval dialog (tool name, args, category)
   → User clicks Approve or Reject
   → ApproveMITL() / RejectMITL() sends response via mitlCh
4. If approved → execute tool
5. If rejected → return "Tool call '{name}' was rejected by the user."
```

**Channel design:**
```go
mitlCh chan mitlResponse // buffered, capacity 1
```

Buffered channel with capacity 1 prevents deadlock if the frontend sends a response before the backend is ready to receive.

## Tool Definition Format (for LLM API)

```go
type Tool struct {
    Type     string       `json:"type"`     // Always "function"
    Function ToolFunction `json:"function"`
}

type ToolFunction struct {
    Name        string      `json:"name"`
    Description string      `json:"description"`
    Parameters  interface{} `json:"parameters"` // JSON Schema
}
```

**Critical**: `required` must be `[]string{}` (empty array), never `nil`. A nil value causes API 400 errors with some servers.

### Tool Count Management

With shell scripts + MCP + built-in tools, the total can exceed 30. gemma-4-26b-a4b degrades with 26+ tool definitions. Mitigations:

1. **Disable toggle**: Users can disable unused tools via Settings → Tools
2. **Disabled tools excluded**: Not sent to LLM in `buildToolDefs()`
3. **Q8 quantization recommended**: Q4_K_M has worse tool-calling accuracy

## Multimodal Image Handling

### Image Flow

```
Input paths:
  Drag & drop → data URL → objstore.SaveDataURL() → ImageEntry{ID}
  Clipboard paste → same
  File picker → same
  Tool artifact → objstore.Save() → ImageEntry{ID}

Storage:
  objstore: {id}.{ext} (binary file + index entry)
  Memory: Record.Images = []ImageEntry{{ID: "a1b2c3"}}
  Frontend: ref cache (not React state)

LLM context:
  Latest image → full base64 data URL with label
  Older images → text reference: "[Past image ID: {id}]"
  Recalled image → __IMAGE_RECALL__{id}__{mime}__ marker → expanded in buildMessages()
```

### Smart Image Recall

The LLM can recall past images via the `view-image` tool:

```
User: "Compare this with the image I shared earlier"
LLM: [view-image] → list-images → finds ID → view-image(id)
→ Returns __IMAGE_RECALL__ marker
→ buildMessages() expands marker to data URL
→ Next LLM call sees the recalled image as visual input
```

## Report Generation

### Flow

```
LLM calls create-report(title, content, filename)
  → Extract image references: ![desc](image:ID)
  → Save each to objstore via saveReportImages()
  → Strip image markdown from content
  → Create Record{Role: "report", Report: &ReportData{Title, Filename}}
  → Emit "chat:report" event with image IDs

Frontend display:
  → Render markdown content
  → Load images by ID via GetImageDataURL()
  → Show as gallery (not inline) below report text
  → Fullscreen overlay: Expand, Copy, Save

Save to file:
  → User selects path via native dialog
  → Each image ID → load from objstore → base64 data URL
  → Append inline: ![Image N](data:image/png;base64,...)
  → Write single self-contained markdown file
```

### Design: Gallery, Not Inline

Images are displayed as a gallery below the report text, not embedded inline in markdown. Reasons:
- ReactMarkdown sanitizes data: URLs by default
- Gallery provides consistent layout regardless of markdown structure
- Lightbox viewer works naturally with gallery items
- Copy/Save actions apply per image
