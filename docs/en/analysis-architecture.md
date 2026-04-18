# Analysis Architecture — shell-agent v0.7.0

> Status: Implemented
> Date: 2026-04-19

## Overview

shell-agent v0.7.0 introduces a data analysis subsystem built on embedded DuckDB. The design follows a **two-layer architecture** — interactive queries run in-process for immediate feedback, while heavy analysis runs as a detached background process that survives application shutdown.

A single binary serves both roles via subcommand routing, eliminating deployment complexity.

## Design Decisions

### Why Embedded DuckDB (not external process)

**Considered alternatives:**
1. **gem-query (external CLI)** — Rejected: tied to Vertex AI Gemini, incompatible with local LLM (OpenAI-compatible API)
2. **Separate analysis CLI** — Rejected: two binaries to build, deploy, and configure; API endpoint duplication
3. **Pure LLM analysis (no database)** — Rejected: unable to handle structured queries, aggregations, or large datasets

**Chosen approach:** Embed DuckDB via `github.com/marcboeker/go-duckdb` (CGO) using `database/sql` interface.

**Rationale:**
- shell-agent already has an LLM client — reuse it for SQL generation
- DuckDB's Go driver is lightweight and embeddable
- `database/sql` is the standard Go interface — no vendor lock-in
- Arrow interface excluded (`no_duckdb_arrow` build tag) — unnecessary overhead for our use case

### Why Single Binary with Subcommand

```
shell-agent              → Wails GUI app (normal launch)
shell-agent analyze ...  → Background analysis CLI (detached process)
```

**Considered alternatives:**
1. **Separate `shell-analyzer` binary** — Two build targets, Makefile changes, .app bundle embedding complexity
2. **In-process goroutine** — Dies when app closes; user explicitly requested analysis survival

**Chosen approach:** Check `os.Args[1]` before `wails.Run()`. If "analyze", route to `analysis.RunCLI()`.

**Rationale:**
- Zero deployment complexity — single .app contains everything
- `wails.Run()` is never called in CLI mode — no GUI overhead
- Same binary means `os.Executable()` works directly for spawning
- `internal/analysis/` package shared trivially (same compilation unit)

### Why Two Layers

```
┌─────────────────────────────────────┐
│  Interactive Layer (in-process)     │
│  DuckDB embedded, LLM via app      │
│  Response: seconds                  │
└──────────────┬──────────────────────┘
               │ spawn (Setsid)
┌──────────────▼──────────────────────┐
│  Background Layer (detached)        │
│  Copied DB, own LLM connection      │
│  Response: minutes to hours         │
└─────────────────────────────────────┘
```

**Why not just in-process?**
- Sliding window analysis over large datasets takes minutes
- User explicitly required: "analysis should continue if Shell Agent is closed"
- Detached process with `Setsid` becomes its own session leader — parent death doesn't propagate SIGHUP

**Why not just background?**
- Simple queries ("show top 10") should return in seconds
- Schema inspection, previews, and suggestions need immediate feedback
- Interactive queries build context for deciding what to analyze in depth

## Architecture

### Data Flow

```
User: "Load sales.csv"
  → load-data tool
  → Engine.LoadCSV()
  → DuckDB: CREATE TABLE AS SELECT * FROM read_csv(...)
  → Return: TableMeta (schema, sample data, row count)

User: "Show revenue by region"
  → query-preview tool
  → PromptBuilder.SQLGenerationPrompt()  [schema + guard + question]
  → LLM (OpenAI API) → SQL
  → Engine.DryRun() → validate
  → Engine.Execute() → QueryResult
  → Return: SQL + formatted results

User: "Analyze security threats in background"
  → analyze-bg tool
  → Copy analysis.duckdb → job directory
  → os.Executable() + "analyze" + flags
  → exec.Command with Setsid (detached)
  → Return: job ID (immediate)

  Background process:
  → Open copied DB
  → Create own LLM client (same API endpoint)
  → Summarizer.Analyze() [sliding window]
  → Write status.json (progress updates)
  → Write findings.json + report.md
  → Exit
```

### Package Structure

```
internal/analysis/
├── types.go          # TableMeta, QueryResult, Finding, AnalysisState, MemoryBudget
├── engine.go         # DuckDB: load, query, schema, export, descriptions
├── prompt.go         # SQL generation, summary, suggest, window analysis prompts
├── summarizer.go     # Sliding window engine, LLMClient interface, token estimation
├── reporter.go       # Markdown report generation (severity-grouped)
├── llmadapter.go     # client.Client → LLMClient adapter
└── cli.go            # RunCLI() entry point, flag parsing, status tracking
```

### Tool Set

| Tool | Layer | Purpose |
|------|-------|---------|
| `load-data` | Interactive | CSV/JSON/JSONL → DuckDB table |
| `describe-data` | Interactive | Schema display + description annotation |
| `query-preview` | Interactive | Natural language → SQL → preview results |
| `query-sql` | Interactive | Direct SQL execution |
| `suggest-analysis` | Interactive | LLM suggests analysis perspectives |
| `quick-summary` | Interactive | Query + LLM summarization |
| `analyze-bg` | Background | Spawn detached analysis process |
| `analysis-status` | Interactive | Poll background job progress |
| `analysis-result` | Interactive | Retrieve completed report |
| `reset-analysis` | Interactive | Drop all tables, reinitialize DB |

### Storage Layout

```
~/Library/Application Support/shell-agent/
├── analysis/
│   ├── analysis.duckdb              # Persistent analysis database
│   ├── job-1713488400000/           # Background job directory
│   │   ├── status.json              # {state, progress, started_at, updated_at}
│   │   ├── analysis.duckdb          # Copied DB (avoids file lock)
│   │   ├── findings.json            # Accumulated findings
│   │   └── report.md                # Generated report
│   └── job-.../
```

## Sliding Window Summarization

### Algorithm

Adapted from data-analyzer project, simplified for shell-agent's context.

```
Input: rows[]string (JSON-encoded records), perspective string

for each window (MaxRecordsPerWindow records, with OverlapRatio overlap):
    1. Build prompt: perspective + previous summary + current findings + data chunk
    2. Wrap data in nlk/guard nonce tags (prompt injection defense)
    3. LLM generates JSON: {summary, new_findings[]}
    4. Parse response (nlk/jsonfix for repair)
    5. Update running summary
    6. Append findings, validate severity
    7. Evict low-priority findings if over MaxFindings
    8. Write status.json checkpoint

If multiple windows processed:
    Generate final report via LLM (merge summaries + all findings)
```

### Finding Eviction Strategy

When findings exceed `MaxFindings` (default: 50):

1. Separate by priority: `critical/high/medium` vs `info/low`
2. Keep all high-priority findings
3. Evict `info/low` in FIFO order (oldest first)
4. If high-priority alone exceeds limit, keep most recent

### Token Budget (Defaults)

| Component | Tokens | Purpose |
|-----------|--------|---------|
| Context limit | 65,536 | Total budget (local LLM) |
| Summary | 10,000 | Running summary cap |
| Findings | 15,000 | Accumulated findings cap |
| Raw data | 8,000 min | Data chunk per window |
| System prompt | 2,000 | Fixed overhead |
| Response | 4,000 | LLM response buffer |

### Token Estimation

Dual strategy (take maximum):
- **Char-based**: `len(text) / 4` — accurate for JSON/structured data
- **Word-based**: CJK chars x2 + ASCII words x1.3 — accurate for natural language

This prevents the 4-5x underestimation that word-only counting causes with JSON data (a lesson from data-analyzer).

## DuckDB Integration

### File Locking

DuckDB uses file-level locking (single writer). The main app holds `analysis.duckdb` open. Background jobs receive a **copy** of the database to avoid conflicts:

```go
// analyze-bg handler
copyFile(a.analysis.DBPath(), jobDBPath)
cmd := exec.Command(selfPath, "analyze", "--db", jobDBPath, ...)
```

### Data Loading

DuckDB's native readers handle format detection:

```sql
-- CSV (auto-detect columns, types, delimiters)
CREATE TABLE t AS SELECT * FROM read_csv('file.csv', auto_detect=true)

-- JSON (array of objects)
CREATE TABLE t AS SELECT * FROM read_json('file.json', auto_detect=true)

-- JSONL (newline-delimited)
CREATE TABLE t AS SELECT * FROM read_json('file.jsonl', format='newline_delimited', auto_detect=true)
```

No manual parsing — DuckDB handles encoding, type inference, and schema creation.

### Build Configuration

```makefile
# Arrow C Data Interface excluded — only database/sql used
wails build -tags no_duckdb_arrow
```

Without this tag, Arrow symbols cause linker errors because Wails' build pipeline doesn't properly link the apache/arrow-go CGO objects.

## Security

### SQL Injection Prevention

- **Prompt-level**: System prompt restricts to SELECT only
- **Identifier sanitization**: `sanitizeIdentifier()` strips non-alphanumeric characters
- **String escaping**: `escapeSQLString()` doubles single quotes
- **DryRun validation**: EXPLAIN before execution catches syntax/semantic errors

### Prompt Injection Defense

All user-provided text (questions, analysis perspectives, data chunks) is wrapped with nlk/guard nonce-tagged XML:

```go
tag := guard.NewTag()
wrapped, _ := tag.Wrap(userInput)
sys := tag.Expand("...instructions referencing <{{DATA_TAG}}>...")
```

This prevents injected instructions within data from being followed by the LLM.

## Performance Characteristics

Measured with gemma-4-26b-a4b on Apple Silicon (M-series):

| Operation | Time | Notes |
|-----------|------|-------|
| Load CSV (20 rows) | <50ms | DuckDB native reader |
| Direct SQL query | <10ms | In-process DuckDB |
| Natural language → SQL | ~5s | One LLM round-trip |
| Quick summary | ~10s | Query + LLM summarization |
| Sliding window (15 rows, 4 windows) | ~28s | 4 LLM calls + final report |
| Analysis suggestions | ~16s | One LLM call with schema context |

Binary size increase: ~15MB (DuckDB embedded library).
