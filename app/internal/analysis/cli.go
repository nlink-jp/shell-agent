package analysis

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nlink-jp/shell-agent/internal/client"
)

// JobStatus represents the state of a background analysis job.
type JobStatus struct {
	State     string    `json:"state"`     // "running", "done", "error"
	Progress  string    `json:"progress"`  // e.g., "3/7 windows"
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Error     string    `json:"error,omitempty"`
}

// RunCLI is the entry point for background analysis mode.
// Called when shell-agent is invoked with "analyze" subcommand.
func RunCLI(args []string) {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)

	dbPath := fs.String("db", "", "Path to DuckDB database file")
	apiEndpoint := fs.String("api", "http://localhost:1234/v1", "OpenAI-compatible API endpoint")
	model := fs.String("model", "gemma-4-26b-a4b", "Model name")
	apiKey := fs.String("api-key", "", "API key (optional; prefer SHELL_AGENT_API_KEY env var to avoid leaking via ps)")
	prompt := fs.String("prompt", "", "Analysis perspective/prompt")
	outputDir := fs.String("output", "", "Output directory for results")
	table := fs.String("table", "", "Table to analyze (default: first table)")
	maxWindows := fs.Int("max-windows", 0, "Max windows to process (0 = unlimited)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		os.Exit(1)
	}

	// Environment variable takes precedence over the --api-key flag so the key
	// never appears in `ps` output when spawned by the main app.
	effectiveAPIKey := *apiKey
	if envKey := os.Getenv("SHELL_AGENT_API_KEY"); envKey != "" {
		effectiveAPIKey = envKey
	}

	if *dbPath == "" || *prompt == "" || *outputDir == "" {
		fmt.Fprintf(os.Stderr, "Usage: shell-agent analyze --db <path> --prompt <text> --output <dir>\n")
		fs.PrintDefaults()
		os.Exit(1)
	}

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output dir: %v\n", err)
		os.Exit(1)
	}

	// Write initial status
	status := &JobStatus{
		State:     "running",
		Progress:  "initializing",
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	writeStatus(*outputDir, status)

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Run analysis
	if err := runAnalysis(ctx, *dbPath, *apiEndpoint, *model, effectiveAPIKey, *prompt, *outputDir, *table, *maxWindows, status); err != nil {
		status.State = "error"
		status.Error = err.Error()
		status.UpdatedAt = time.Now()
		writeStatus(*outputDir, status)
		fmt.Fprintf(os.Stderr, "analysis failed: %v\n", err)
		os.Exit(1)
	}

	status.State = "done"
	status.UpdatedAt = time.Now()
	writeStatus(*outputDir, status)
}

func runAnalysis(
	ctx context.Context,
	dbPath, apiEndpoint, model, apiKey, prompt, outputDir, tableName string,
	maxWindows int,
	status *JobStatus,
) error {
	// Open database
	engine, err := NewEngine(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer engine.Close()

	// Load schema
	schema, err := engine.LoadSchema(ctx)
	if err != nil {
		return fmt.Errorf("load schema: %w", err)
	}

	// Determine target table
	if tableName == "" {
		// Use first table from schema
		rows, err := engine.DB().QueryContext(ctx, "SHOW TABLES")
		if err != nil {
			return fmt.Errorf("list tables: %w", err)
		}
		defer rows.Close()
		if rows.Next() {
			rows.Scan(&tableName)
		}
		if tableName == "" {
			return fmt.Errorf("no tables found in database")
		}
	}

	// Get all data as rows
	result, err := engine.Execute(ctx, fmt.Sprintf("SELECT * FROM %s", sanitizeIdentifier(tableName)))
	if err != nil {
		return fmt.Errorf("read table: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("query error: %s", result.Error)
	}

	dataRows := ResultToRows(result)

	status.Progress = fmt.Sprintf("loaded %d rows from %s", len(dataRows), tableName)
	status.UpdatedAt = time.Now()
	writeStatus(outputDir, status)

	// Create LLM client
	llmClient := client.New(apiEndpoint, model, apiKey)
	adapter := NewClientAdapter(llmClient)

	// Create summarizer
	builder := NewPromptBuilder(schema)
	cfg := DefaultSummarizerConfig()
	summarizer := NewSummarizer(adapter, builder, cfg)

	// Run analysis with status callback
	windowCount := 0
	analyzeResult, err := summarizer.Analyze(ctx, prompt, dataRows, func(windowIdx, totalWindows int, msg string) {
		windowCount = windowIdx + 1
		if maxWindows > 0 && windowCount > maxWindows {
			return
		}
		status.Progress = msg
		status.UpdatedAt = time.Now()
		writeStatus(outputDir, status)
	})
	if err != nil {
		return fmt.Errorf("analyze: %w", err)
	}

	// Write findings
	findingsData, _ := json.MarshalIndent(analyzeResult.Findings, "", "  ")
	if err := os.WriteFile(filepath.Join(outputDir, "findings.json"), findingsData, 0o644); err != nil {
		return fmt.Errorf("write findings: %w", err)
	}

	// Generate and write report
	reporter := NewReportGenerator()
	report := reporter.GenerateResultReport(prompt, analyzeResult)
	if err := os.WriteFile(filepath.Join(outputDir, "report.md"), []byte(report), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	status.Progress = fmt.Sprintf("completed: %d windows, %d findings", analyzeResult.Windows, len(analyzeResult.Findings))
	return nil
}

func writeStatus(dir string, status *JobStatus) {
	data, _ := json.MarshalIndent(status, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "status.json"), data, 0o644)
}
