//go:build live

// Run with: go test -tags live -v -run TestIntegration -count=1 -timeout 5m

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nlink-jp/shell-agent/internal/analysis"
	"github.com/nlink-jp/shell-agent/internal/client"
	"github.com/nlink-jp/shell-agent/internal/config"
)

// newTestApp creates a minimal App with analysis engine and LLM client for integration testing.
func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	analysisDir := filepath.Join(dir, "analysis")
	dbPath := filepath.Join(analysisDir, "analysis.duckdb")

	eng, err := analysis.NewEngine(dbPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	app := &App{
		llm:         client.New("http://localhost:1234/v1", "google/gemma-4-26b-a4b", ""),
		analysis:    eng,
		analysisDir: analysisDir,
		cfg: &config.Config{
			API: config.APIConfig{
				Endpoint: "http://localhost:1234/v1",
				Model:    "google/gemma-4-26b-a4b",
			},
		},
		mitlCh: make(chan mitlResponse, 1),
	}

	t.Cleanup(func() {
		eng.Close()
	})

	return app
}

func TestIntegration_LoadAndDescribe(t *testing.T) {
	if _, err := os.Stat("/tmp/shell-agent-e2e/sales.csv"); err != nil {
		t.Skip("test data not found")
	}
	app := newTestApp(t)

	// --- load-data: CSV ---
	t.Log("=== load-data (CSV) ===")
	result := app.loadDataTool(`{"file_path":"/tmp/shell-agent-e2e/sales.csv","table_name":"sales"}`)
	t.Log(result)
	if strings.Contains(result, "Error") {
		t.Fatalf("load-data failed: %s", result)
	}
	if !strings.Contains(result, "20 rows") {
		t.Error("expected 20 rows in result")
	}

	// --- load-data: JSONL ---
	t.Log("=== load-data (JSONL) ===")
	result = app.loadDataTool(`{"file_path":"/tmp/shell-agent-e2e/access_logs.jsonl","table_name":"access_logs"}`)
	t.Log(result)
	if strings.Contains(result, "Error") {
		t.Fatalf("load-data JSONL failed: %s", result)
	}
	if !strings.Contains(result, "15 rows") {
		t.Error("expected 15 rows in result")
	}

	// --- describe-data: no args (list all) ---
	t.Log("=== describe-data (list all) ===")
	result = app.describeDataTool(`{}`)
	t.Log(result)
	if !strings.Contains(result, "sales") || !strings.Contains(result, "access_logs") {
		t.Error("describe-data should list both tables")
	}

	// --- describe-data: set descriptions ---
	t.Log("=== describe-data (set descriptions) ===")
	result = app.describeDataTool(`{"table_name":"sales","description":"Weekly product sales","columns":{"total":"Revenue in JPY"}}`)
	t.Log(result)
	if !strings.Contains(result, "Descriptions updated") {
		t.Error("expected descriptions updated")
	}

	// --- describe-data: verify descriptions appear in schema ---
	result = app.describeDataTool(`{"table_name":"sales"}`)
	t.Log(result)
	if !strings.Contains(result, "Weekly product sales") {
		t.Error("table description not persisted")
	}
	if !strings.Contains(result, "Revenue in JPY") {
		t.Error("column description not persisted")
	}
}

func TestIntegration_QuerySQL(t *testing.T) {
	if _, err := os.Stat("/tmp/shell-agent-e2e/sales.csv"); err != nil {
		t.Skip("test data not found")
	}
	app := newTestApp(t)
	app.loadDataTool(`{"file_path":"/tmp/shell-agent-e2e/sales.csv","table_name":"sales"}`)

	// --- query-sql: direct SQL ---
	t.Log("=== query-sql ===")
	result := app.querySQLTool(`{"sql":"SELECT region, SUM(total) as revenue FROM sales GROUP BY region ORDER BY revenue DESC"}`)
	t.Log(result)
	if strings.Contains(result, "Error") {
		t.Fatalf("query-sql failed: %s", result)
	}
	if !strings.Contains(result, "Tokyo") {
		t.Error("result should contain Tokyo")
	}

	// --- query-sql: invalid SQL ---
	t.Log("=== query-sql (invalid) ===")
	result = app.querySQLTool(`{"sql":"SELEC * FORM sales"}`)
	t.Log(result)
	if !strings.Contains(result, "error") && !strings.Contains(result, "Error") {
		t.Error("should report error for invalid SQL")
	}
}

func TestIntegration_QueryPreview(t *testing.T) {
	if _, err := os.Stat("/tmp/shell-agent-e2e/sales.csv"); err != nil {
		t.Skip("test data not found")
	}
	app := newTestApp(t)
	app.loadDataTool(`{"file_path":"/tmp/shell-agent-e2e/sales.csv","table_name":"sales"}`)

	// --- query-preview: natural language → SQL → execute ---
	t.Log("=== query-preview ===")
	result := app.queryPreviewTool(`{"question":"What is the total revenue per product?","limit":10}`)
	t.Log(result)
	if strings.Contains(result, "Error") {
		t.Fatalf("query-preview failed: %s", result)
	}
	if !strings.Contains(result, "SQL:") {
		t.Error("result should show generated SQL")
	}
}

func TestIntegration_QuickSummary(t *testing.T) {
	if _, err := os.Stat("/tmp/shell-agent-e2e/sales.csv"); err != nil {
		t.Skip("test data not found")
	}
	app := newTestApp(t)
	app.loadDataTool(`{"file_path":"/tmp/shell-agent-e2e/sales.csv","table_name":"sales"}`)

	// --- quick-summary with direct SQL ---
	t.Log("=== quick-summary (SQL) ===")
	result := app.quickSummaryTool(`{"sql":"SELECT product, SUM(total) as revenue FROM sales GROUP BY product ORDER BY revenue DESC"}`)
	t.Log(result)
	if strings.Contains(result, "Error") {
		t.Fatalf("quick-summary failed: %s", result)
	}
	if !strings.Contains(result, "Summary") {
		t.Error("result should contain summary")
	}

	// --- quick-summary with natural language ---
	t.Log("=== quick-summary (question) ===")
	result = app.quickSummaryTool(`{"question":"Which region has the highest revenue?"}`)
	t.Log(result)
	if strings.Contains(result, "Error") {
		t.Fatalf("quick-summary question failed: %s", result)
	}
}

func TestIntegration_SuggestAnalysis(t *testing.T) {
	if _, err := os.Stat("/tmp/shell-agent-e2e/sales.csv"); err != nil {
		t.Skip("test data not found")
	}
	app := newTestApp(t)
	app.loadDataTool(`{"file_path":"/tmp/shell-agent-e2e/sales.csv","table_name":"sales"}`)
	app.loadDataTool(`{"file_path":"/tmp/shell-agent-e2e/access_logs.jsonl","table_name":"access_logs"}`)

	t.Log("=== suggest-analysis ===")
	result := app.suggestAnalysisTool()
	t.Log(result)
	if strings.Contains(result, "Error") {
		t.Fatalf("suggest-analysis failed: %s", result)
	}
	if len(result) < 100 {
		t.Error("suggestions seem too short")
	}
}

func TestIntegration_ResetAnalysis(t *testing.T) {
	if _, err := os.Stat("/tmp/shell-agent-e2e/sales.csv"); err != nil {
		t.Skip("test data not found")
	}
	app := newTestApp(t)
	app.loadDataTool(`{"file_path":"/tmp/shell-agent-e2e/sales.csv","table_name":"sales"}`)

	// Verify data exists
	result := app.describeDataTool(`{}`)
	if !strings.Contains(result, "sales") {
		t.Fatal("sales table should exist")
	}

	// Reset
	t.Log("=== reset-analysis ===")
	result = app.resetAnalysisTool()
	t.Log(result)
	if !strings.Contains(result, "reset") && !strings.Contains(result, "Reset") {
		t.Error("expected reset confirmation")
	}

	// Verify empty
	result = app.describeDataTool(`{}`)
	t.Log("After reset:", result)
	if strings.Contains(result, "sales") {
		t.Error("sales table should be gone after reset")
	}
}

func TestIntegration_AnalysisStatusEmpty(t *testing.T) {
	app := newTestApp(t)

	t.Log("=== analysis-status (empty) ===")
	result := app.analysisStatusTool(`{}`)
	t.Log(result)
	if !strings.Contains(result, "No analysis jobs") {
		t.Error("should report no jobs")
	}
}

func TestIntegration_AnalysisResultMissing(t *testing.T) {
	app := newTestApp(t)

	t.Log("=== analysis-result (missing) ===")
	result := app.analysisResultTool(`{"job_id":"nonexistent-job"}`)
	t.Log(result)
	if !strings.Contains(result, "not found") {
		t.Error("should report not found")
	}
}

func TestIntegration_HandleBuiltinToolRouting(t *testing.T) {
	app := newTestApp(t)

	// Verify all analysis tools are routed through handleBuiltinTool
	analysisToolNames := []string{
		"load-data", "describe-data", "query-preview", "query-sql",
		"suggest-analysis", "quick-summary", "analyze-bg",
		"analysis-status", "analysis-result", "reset-analysis",
	}

	for _, name := range analysisToolNames {
		_, handled := app.handleBuiltinTool(name, "{}")
		if !handled {
			t.Errorf("tool %q should be handled by handleBuiltinTool", name)
		}
	}

	// Non-analysis tool should not be handled
	_, handled := app.handleBuiltinTool("nonexistent-tool", "{}")
	if handled {
		t.Error("nonexistent tool should not be handled")
	}
}

func TestIntegration_ToolDefinitions(t *testing.T) {
	app := newTestApp(t)

	tools := app.analysisTools()
	t.Logf("Analysis tool count: %d", len(tools))

	if len(tools) != 10 {
		t.Errorf("expected 10 analysis tools, got %d", len(tools))
	}

	// Verify each tool has required fields
	for _, tool := range tools {
		if tool.Function.Name == "" {
			t.Error("tool has empty name")
		}
		if tool.Function.Description == "" {
			t.Errorf("tool %q has empty description", tool.Function.Name)
		}
		if tool.Function.Parameters == nil {
			t.Errorf("tool %q has nil parameters", tool.Function.Name)
		}

		// Verify parameters is valid JSON
		params, ok := tool.Function.Parameters.(map[string]any)
		if !ok {
			t.Errorf("tool %q parameters not a map", tool.Function.Name)
			continue
		}
		if params["type"] != "object" {
			t.Errorf("tool %q parameters type should be 'object'", tool.Function.Name)
		}

		// Verify required is not nil (prevents API 400 errors)
		req, ok := params["required"]
		if !ok {
			t.Errorf("tool %q missing 'required' field", tool.Function.Name)
		}
		if req == nil {
			t.Errorf("tool %q 'required' is nil (must be empty array)", tool.Function.Name)
		}
	}

	// Verify tool definitions can be serialized (sent to LLM API)
	data, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("failed to marshal tools: %v", err)
	}
	t.Logf("Serialized tool definitions: %d bytes", len(data))
}
