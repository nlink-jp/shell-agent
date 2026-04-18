//go:build live

// Run with: go test ./internal/analysis/ -tags live -v -run TestLive -count=1

package analysis

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nlink-jp/shell-agent/internal/client"
)

const (
	liveAPIEndpoint = "http://localhost:1234/v1"
	liveModel       = "google/gemma-4-26b-a4b"
)

func newLiveAdapter(t *testing.T) *ClientAdapter {
	t.Helper()
	c := client.New(liveAPIEndpoint, liveModel, "")
	return NewClientAdapter(c)
}

func TestLive_SQLGeneration(t *testing.T) {
	csvPath := "/tmp/shell-agent-e2e/sales.csv"
	if _, err := os.Stat(csvPath); err != nil {
		t.Skip("test data not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Load data
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	_, err = eng.LoadCSV(ctx, csvPath, "sales")
	if err != nil {
		t.Fatalf("LoadCSV: %v", err)
	}

	schema, _ := eng.LoadSchema(ctx)
	builder := NewPromptBuilder(schema)
	adapter := newLiveAdapter(t)

	// Test 1: Simple aggregation
	t.Log("--- Test 1: SQL generation (aggregation) ---")
	sys, user, err := builder.SQLGenerationPrompt("Show total revenue by region")
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}

	resp, err := adapter.Chat(ctx, sys, user)
	if err != nil {
		t.Fatalf("LLM chat: %v", err)
	}
	sql1 := CleanSQL(resp)
	t.Logf("Generated SQL: %s", sql1)

	if !strings.Contains(strings.ToUpper(sql1), "SELECT") {
		t.Errorf("response should be SQL, got: %s", sql1)
	}

	// Validate with DryRun
	if err := eng.DryRun(ctx, sql1); err != nil {
		t.Errorf("DryRun failed: %v\nSQL: %s", err, sql1)
	} else {
		// Execute
		result, err := eng.Execute(ctx, sql1)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.Error != "" {
			t.Errorf("query error: %s", result.Error)
		} else {
			t.Logf("Result: %d rows\n%s", result.RowCount, FormatResultSummary(result))
		}
	}

	// Test 2: Filtering
	t.Log("--- Test 2: SQL generation (filtering) ---")
	sys2, user2, _ := builder.SQLGenerationPrompt("Which product sold the most units in Tokyo?")
	resp2, err := adapter.Chat(ctx, sys2, user2)
	if err != nil {
		t.Fatalf("LLM chat: %v", err)
	}
	sql2 := CleanSQL(resp2)
	t.Logf("Generated SQL: %s", sql2)

	if err := eng.DryRun(ctx, sql2); err != nil {
		t.Errorf("DryRun failed: %v\nSQL: %s", err, sql2)
	} else {
		result, _ := eng.Execute(ctx, sql2)
		if result != nil && result.Error == "" {
			t.Logf("Result: %s", FormatResultSummary(result))
		}
	}
}

func TestLive_QuickSummary(t *testing.T) {
	csvPath := "/tmp/shell-agent-e2e/sales.csv"
	if _, err := os.Stat(csvPath); err != nil {
		t.Skip("test data not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	_, _ = eng.LoadCSV(ctx, csvPath, "sales")
	schema, _ := eng.LoadSchema(ctx)

	builder := NewPromptBuilder(schema)
	adapter := newLiveAdapter(t)
	summarizer := NewSummarizer(adapter, builder, DefaultSummarizerConfig())

	// Execute a query and summarize
	result, err := eng.Execute(ctx, "SELECT region, product, SUM(total) as revenue, SUM(quantity) as qty FROM sales GROUP BY region, product ORDER BY revenue DESC")
	if err != nil || result.Error != "" {
		t.Fatalf("query: %v / %s", err, result.Error)
	}

	t.Log("--- Summarizing query result ---")
	t.Logf("Data: %d rows", result.RowCount)

	summary, err := summarizer.SummarizeResult(ctx, result)
	if err != nil {
		t.Fatalf("SummarizeResult: %v", err)
	}
	t.Logf("Summary:\n%s", summary)

	if summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestLive_SuggestAnalysis(t *testing.T) {
	csvPath := "/tmp/shell-agent-e2e/sales.csv"
	jsonlPath := "/tmp/shell-agent-e2e/access_logs.jsonl"
	if _, err := os.Stat(csvPath); err != nil {
		t.Skip("test data not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	_, _ = eng.LoadCSV(ctx, csvPath, "sales")
	_, _ = eng.LoadJSONL(ctx, jsonlPath, "access_logs")
	eng.SetTableDescription("sales", "Weekly product sales by region")
	eng.SetTableDescription("access_logs", "HTTP access logs from API gateway")

	schema, _ := eng.LoadSchema(ctx)
	builder := NewPromptBuilder(schema)
	adapter := newLiveAdapter(t)
	summarizer := NewSummarizer(adapter, builder, DefaultSummarizerConfig())

	t.Log("--- Suggesting analysis perspectives ---")
	suggestions, err := summarizer.SuggestAnalysis(ctx, eng.Tables())
	if err != nil {
		t.Fatalf("SuggestAnalysis: %v", err)
	}
	t.Logf("Suggestions:\n%s", suggestions)

	if suggestions == "" {
		t.Error("suggestions should not be empty")
	}
}

func TestLive_SlidingWindowAnalysis(t *testing.T) {
	jsonlPath := "/tmp/shell-agent-e2e/access_logs.jsonl"
	if _, err := os.Stat(jsonlPath); err != nil {
		t.Skip("test data not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	_, _ = eng.LoadJSONL(ctx, jsonlPath, "access_logs")
	schema, _ := eng.LoadSchema(ctx)

	// Get all rows
	result, err := eng.Execute(ctx, "SELECT * FROM access_logs ORDER BY timestamp")
	if err != nil || result.Error != "" {
		t.Fatalf("query: %v / %s", err, result.Error)
	}
	rows := ResultToRows(result)
	t.Logf("Total rows: %d", len(rows))

	builder := NewPromptBuilder(schema)
	adapter := newLiveAdapter(t)
	cfg := DefaultSummarizerConfig()
	cfg.MaxRecordsPerWindow = 5
	cfg.OverlapRatio = 0.2
	summarizer := NewSummarizer(adapter, builder, cfg)

	t.Log("--- Running sliding window analysis ---")
	analyzeResult, err := summarizer.Analyze(ctx,
		"Detect security threats: brute force attacks, path traversal, unauthorized access attempts, and suspicious IP patterns",
		rows,
		func(idx, total int, status string) {
			t.Logf("  [%d/%d] %s", idx+1, total, status)
		},
	)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	t.Logf("Windows processed: %d", analyzeResult.Windows)
	t.Logf("Findings: %d", len(analyzeResult.Findings))
	t.Logf("Duration: %s", analyzeResult.Duration.Round(time.Second))
	t.Logf("Summary:\n%s", analyzeResult.Summary)

	for i, f := range analyzeResult.Findings {
		t.Logf("  Finding %d: [%s] %s", i+1, f.Severity, f.Description)
	}

	if analyzeResult.Summary == "" {
		t.Error("summary should not be empty")
	}

	// Generate report
	reporter := NewReportGenerator()
	report := reporter.GenerateResultReport("Security Threat Analysis", analyzeResult)
	t.Logf("\n--- Generated Report ---\n%s", report)

	reportPath := fmt.Sprintf("/tmp/shell-agent-e2e/report_%d.md", time.Now().Unix())
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err == nil {
		t.Logf("Report saved: %s", reportPath)
	}
}
