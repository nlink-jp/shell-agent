# Memory Architecture — shell-agent

> Status: v0.7.0
> Date: 2026-04-19

## Overview

shell-agent's memory system provides **time-aware, multi-tier conversation persistence** designed for local LLM constraints. Unlike cloud-based assistants with large context windows, local models typically operate within 8K-128K tokens. The memory system ensures long conversations remain coherent by progressively summarizing older context while preserving temporal awareness.

## Design Decisions

### Why Three Tiers (not RAG, not fixed window)

**Considered alternatives:**
1. **Fixed sliding window** — Rejected: abrupt context loss, no summary of what was discussed
2. **RAG (Retrieval-Augmented Generation)** — Rejected: requires embedding model, vector store, and retrieval pipeline — too heavy for local-only tool
3. **Unlimited context** — Rejected: local LLMs have hard limits; even 128K models degrade with full context

**Chosen approach:** Hot/Warm/Cold tiers with LLM-powered compaction.

**Rationale:**
- Hot tier preserves exact conversation for immediate context
- Warm summaries retain key decisions and topics from recent history
- Cold summaries provide long-term session continuity
- LLM generates summaries — leveraging the same model for compression
- Timestamps embedded in every record enable temporal reasoning

### Why Pinned Memory (not just summaries)

Warm/cold summaries are temporal — they capture "what was discussed." But some facts are **timeless**: user preferences, important decisions, learned context. Pinned Memory extracts these facts autonomously and persists them across sessions.

**Why bilingual:** The system prompt is in English (better LLM reasoning). But facts like "ユーザーはVimを好む" need native language for the user's context. Both forms are stored: English for system prompt injection, native for display.

## Architecture

### Record Structure

Every message in the system is a `Record`:

```go
type Record struct {
    Timestamp    time.Time      // Wall clock time (injected by system)
    Role         string         // "user", "assistant", "tool", "system", "report"
    Content      string         // Full text content
    Tier         Tier           // hot | warm | cold
    SummaryRange *TimeRange     // For warm/cold: time span of summarized content
    Images       []ImageEntry   // objstore ID references
    InTokens     int            // Prompt tokens consumed
    OutTokens    int            // Completion tokens generated
    Report       *ReportData    // Non-nil if role == "report"
}
```

Key design: **timestamps are system-injected, not LLM-generated.** The LLM sees `[15:04:05]` prefixed to each message but doesn't control the clock. This enables accurate temporal reasoning ("what did I say 10 minutes ago?").

### Tier Lifecycle

```
User sends message
  ↓
Hot Tier ← new Record{Tier: hot}
  │
  │ HotTokenCount() > HotTokenLimit (65536)?
  │ Yes ↓
  │
  PromoteOldestHotToWarm()
  │ → Select oldest hot records (keep latest user+assistant pair)
  │ → Target: reduce by excess tokens
  ↓
  LLM summarizes selected records
  ↓
Warm Tier ← new Record{Tier: warm, SummaryRange: {from, to}}
  │
  │ Record older than WarmRetentionMins (60)?
  │ Yes ↓
  │
Cold Tier ← reclassified Record{Tier: cold}
  │
  │ Record older than ColdRetentionMins (1440)?
  │ Yes ↓
  │
  Discarded (summary persists in session JSON)
```

### Hot Tier — Verbatim Conversation

- **Content**: Full message text, unmodified
- **Token budget**: `cfg.Memory.HotTokenLimit` (default 65,536)
- **Token counting**: `EstimateTokens()` — dual strategy:
  - `len(text) / 2` (char-based, conservative)
  - `len(text) / 4` (word-based, for English)
  - Takes maximum to prevent undercount
- **Auto-correction**: HotTokenLimit below 8192 is raised to 8192 (prevents aggressive compaction that deletes user context)
- **Ordering**: Always chronological within tier

### Warm Tier — LLM Summaries

- **Content**: LLM-generated summary of compacted hot messages
- **TimeRange**: `{From, To}` — timestamps of original messages
- **Retention**: `cfg.Memory.WarmRetentionMins` (default 60 minutes)
- **System prompt injection**: Warm summaries appear as system messages in LLM context:
  ```
  [Previous conversation summary (15:00-15:30)]:
  User discussed project architecture. Decided to use DuckDB for analysis.
  Key topics: data loading, SQL generation, background processing.
  ```

### Cold Tier — Archival Summaries

- **Content**: Same format as warm, but from earlier in the session
- **Retention**: `cfg.Memory.ColdRetentionMins` (default 1440 = 24 hours)
- **Purpose**: Provides deep context for very long sessions

### Compaction Algorithm

```go
func (a *App) compactMemoryIfNeeded() {
    hotCount := a.session.HotTokenCount()
    if hotCount <= a.cfg.Memory.HotTokenLimit {
        return // No compaction needed
    }

    // How many tokens to shed
    excess := hotCount - a.cfg.Memory.HotTokenLimit

    // Select oldest hot records for promotion
    // Always keep at least the latest user + assistant pair
    promoted := a.session.PromoteOldestHotToWarm(excess)

    // Generate summary via LLM
    summaryPrompt := buildSummarizationPrompt(promoted)
    summary := a.llm.Chat(summaryPrompt)

    // Replace promoted records with single warm summary
    a.session.ApplySummary(promoted, summary)
}
```

**Summarization prompt**: Instructs LLM to preserve key decisions, action items, and temporal references while condensing the conversation.

### Message Building for LLM

The `buildMessages()` function assembles the LLM context in this order:

```
1. System prompt
   ├── Current time + timezone ("2026-04-19 15:04:05 JST UTC+09:00")
   ├── Location (if available)
   ├── Pinned memories (English facts)
   └── Guard tag instructions

2. Cold summaries (chronological)

3. Warm summaries (chronological)

4. Hot messages (chronological)
   ├── User messages with [HH:MM:SS] prefix
   ├── Assistant responses
   ├── Tool results (role: "user" for API, but tagged as tool)
   └── Reports (truncated to 200 chars)

5. Latest user message
   └── With current image as data URL (if multimodal)
```

## Pinned Memory

### Extraction Process

After each assistant turn, the system extracts important facts:

```go
func (a *App) extractPinnedMemories() {
    // Take last 4 hot messages for analysis
    recent := lastN(a.session.HotRecords(), 4)

    // Ask LLM to identify pinnable facts
    prompt := buildPinnedExtractionPrompt(recent, existingPinned)

    // LLM returns JSON: [{fact, native_fact, category}]
    newPinned := parsePinnedResponse(llmResponse)

    // Deduplicate against existing
    for _, p := range newPinned {
        if !isDuplicate(p, existingPinned) {
            a.pinned.Add(p)
        }
    }
}
```

### Pinned Memory Structure

```go
type PinnedMemory struct {
    Fact       string    // English: "User prefers Vim over VS Code"
    NativeFact string    // Native: "ユーザーはVimを好む"
    Category   string    // "preference" | "decision" | "fact" | "context"
    SourceTime time.Time // When the fact was mentioned
    CreatedAt  time.Time // When it was pinned
}
```

### Categories

| Category | Purpose | Example |
|----------|---------|---------|
| `preference` | User preferences and habits | "Prefers dark theme" |
| `decision` | Architectural or design decisions | "Chose DuckDB over SQLite" |
| `fact` | Domain knowledge or context | "Project uses Wails v2" |
| `context` | Situational awareness | "User is a security engineer" |

### Persistence

- Stored in `~/Library/Application Support/shell-agent/pinned.json`
- Loaded at startup, injected into every system prompt
- Editable via Status panel (double-click to edit, delete with confirmation)
- Survives across all sessions

## Time and Space Awareness

### Time Injection

Every system prompt includes:
```
Current time: 2026-04-19 15:04:05 (timezone: JST, UTC+09:00)
```

Every user message in memory is prefixed:
```
[15:04:05] User's actual message here
```

This enables the LLM to:
- Understand elapsed time between messages
- Resolve relative time references ("what I said 30 minutes ago")
- Generate time-appropriate responses ("good morning" vs "good evening")

**Timestamp leaking prevention**: LLMs sometimes mimic the `[HH:MM:SS]` format in their responses. `stripLeakedTimestamps()` removes these before display.

### Space Injection

Location data (when available via get-location tool):
```
Location: Tokyo, Japan (timezone-based inference)
```

Enables location-aware responses without external GPS API.

## Session Persistence

### Session Structure

```go
type Session struct {
    ID        string    // "session-{unixMilli}"
    Title     string    // Auto-generated or user-renamed
    CreatedAt time.Time
    UpdatedAt time.Time
    Records   []Record  // Complete conversation history (all tiers)
}
```

### Storage Format

- File: `{ConfigDir}/sessions/{id}.json`
- All records serialized, including warm/cold summaries
- Image data: objstore IDs only (not base64)
- Token counts preserved per message
- Report metadata included

### Lifecycle

1. **New session**: Created on app start (or resumed if StartupMode == "last")
2. **Auto-save**: After every message exchange
3. **Title generation**: From first user message content
4. **Restore**: Full record history loaded, images lazy-loaded from objstore
5. **Session switching**: Current session saved, new session loaded
6. **Shutdown**: Final save with window state

## Token Budget Management

### Defaults

| Parameter | Default | Purpose |
|-----------|---------|---------|
| `HotTokenLimit` | 65,536 | Maximum hot tier tokens |
| `WarmRetentionMins` | 60 | Minutes before warm → cold |
| `ColdRetentionMins` | 1440 | Minutes before cold → discard |
| `MaxToolRounds` | 10 | Max tool call iterations per turn |

### Auto-Correction

If `HotTokenLimit < 8192`, it's raised to 8192. This prevents a misconfiguration where a single tool result (e.g., web search) consumes the entire budget and compaction immediately deletes the user's message.

### Token Estimation

```go
func EstimateTokens(text string) int {
    charBased := len(text) / 2  // Conservative for CJK
    wordBased := len(text) / 4  // Standard for English
    return max(charBased, wordBased)
}
```

Dual strategy prevents underestimation for JSON-heavy content (lesson from data-analyzer project).
