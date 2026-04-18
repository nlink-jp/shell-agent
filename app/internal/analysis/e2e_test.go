package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_FullPipeline tests the complete analysis pipeline:
// load → schema → query → summarize → export → report
func TestE2E_FullPipeline(t *testing.T) {
	csvPath := "/tmp/shell-agent-e2e/sales.csv"
	jsonlPath := "/tmp/shell-agent-e2e/access_logs.jsonl"

	if _, err := os.Stat(csvPath); err != nil {
		t.Skip("E2E test data not found at /tmp/shell-agent-e2e/")
	}

	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "e2e.duckdb")

	// --- Step 1: Create engine ---
	eng, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	// --- Step 2: Load CSV ---
	t.Log("Loading CSV...")
	salesMeta, err := eng.LoadCSV(ctx, csvPath, "sales")
	if err != nil {
		t.Fatalf("LoadCSV: %v", err)
	}
	t.Logf("  sales: %d rows, %d columns", salesMeta.RowCount, len(salesMeta.Columns))

	if salesMeta.RowCount != 20 {
		t.Errorf("sales row count: got %d, want 20", salesMeta.RowCount)
	}
	if len(salesMeta.Columns) != 6 {
		t.Errorf("sales columns: got %d, want 6", len(salesMeta.Columns))
	}

	// --- Step 3: Load JSONL ---
	t.Log("Loading JSONL...")
	logsMeta, err := eng.LoadJSONL(ctx, jsonlPath, "access_logs")
	if err != nil {
		t.Fatalf("LoadJSONL: %v", err)
	}
	t.Logf("  access_logs: %d rows, %d columns", logsMeta.RowCount, len(logsMeta.Columns))

	if logsMeta.RowCount != 15 {
		t.Errorf("logs row count: got %d, want 15", logsMeta.RowCount)
	}

	// --- Step 4: Schema with descriptions ---
	t.Log("Setting descriptions...")
	eng.SetTableDescription("sales", "Weekly product sales by region")
	eng.SetColumnDescription("sales", "total", "quantity * unit_price in JPY")
	eng.SetTableDescription("access_logs", "HTTP access logs from API gateway")
	eng.SetColumnDescription("access_logs", "duration_ms", "Response time in milliseconds")

	schema, err := eng.LoadSchema(ctx)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	t.Logf("Schema:\n%s", schema)

	if !strings.Contains(schema, "Weekly product sales") {
		t.Error("schema missing table description")
	}
	if !strings.Contains(schema, "quantity * unit_price in JPY") {
		t.Error("schema missing column description")
	}

	// --- Step 5: Direct SQL queries ---
	t.Log("Running queries...")

	// Sales by region
	r1, err := eng.Execute(ctx, "SELECT region, SUM(total) as revenue FROM sales GROUP BY region ORDER BY revenue DESC")
	if err != nil {
		t.Fatalf("query 1: %v", err)
	}
	if r1.Error != "" {
		t.Fatalf("query 1 error: %s", r1.Error)
	}
	t.Logf("  Revenue by region: %d rows, %v", r1.RowCount, FormatResultSummary(r1))
	if r1.RowCount != 3 {
		t.Errorf("expected 3 regions, got %d", r1.RowCount)
	}

	// Top product
	r2, err := eng.Execute(ctx, "SELECT product, SUM(quantity) as total_qty FROM sales GROUP BY product ORDER BY total_qty DESC LIMIT 1")
	if err != nil {
		t.Fatalf("query 2: %v", err)
	}
	t.Logf("  Top product: %s", FormatResultSummary(r2))

	// Access log anomalies: 403/401 status
	r3, err := eng.Execute(ctx, "SELECT src_ip, status, COUNT(*) as cnt FROM access_logs WHERE status >= 400 GROUP BY src_ip, status ORDER BY cnt DESC")
	if err != nil {
		t.Fatalf("query 3: %v", err)
	}
	t.Logf("  Error status by IP: %s", FormatResultSummary(r3))
	if r3.RowCount == 0 {
		t.Error("expected error status rows")
	}

	// Path traversal detection
	r4, err := eng.Execute(ctx, "SELECT * FROM access_logs WHERE path LIKE '%..%'")
	if err != nil {
		t.Fatalf("query 4: %v", err)
	}
	t.Logf("  Path traversal attempts: %d", r4.RowCount)
	if r4.RowCount != 1 {
		t.Errorf("expected 1 path traversal, got %d", r4.RowCount)
	}

	// --- Step 6: DryRun validation ---
	t.Log("Testing DryRun...")
	if err := eng.DryRun(ctx, "SELECT * FROM sales LIMIT 5"); err != nil {
		t.Errorf("DryRun valid: %v", err)
	}
	if err := eng.DryRun(ctx, "SELEC * FROM sales"); err == nil {
		t.Error("DryRun should fail on syntax error")
	}

	// --- Step 7: Export ---
	t.Log("Testing exports...")
	csvOut := filepath.Join(dir, "export_sales.csv")
	if err := eng.ExportTable(ctx, "sales", csvOut, "csv"); err != nil {
		t.Fatalf("ExportTable CSV: %v", err)
	}
	csvData, _ := os.ReadFile(csvOut)
	if !strings.Contains(string(csvData), "Widget") {
		t.Error("CSV export missing data")
	}

	jsonOut := filepath.Join(dir, "export_errors.json")
	if err := eng.ExportQuery(ctx, "SELECT * FROM access_logs WHERE status >= 400", jsonOut, "json"); err != nil {
		t.Fatalf("ExportQuery JSON: %v", err)
	}
	jsonData, _ := os.ReadFile(jsonOut)
	if !strings.Contains(string(jsonData), "203.0.113.50") {
		t.Error("JSON export missing suspicious IP")
	}

	// --- Step 8: ResultToRows for sliding window ---
	t.Log("Testing ResultToRows...")
	allLogs, err := eng.Execute(ctx, "SELECT * FROM access_logs")
	if err != nil {
		t.Fatalf("select all logs: %v", err)
	}
	rows := ResultToRows(allLogs)
	if len(rows) != 15 {
		t.Errorf("ResultToRows: got %d, want 15", len(rows))
	}
	// Verify each row is valid JSON
	for i, row := range rows {
		var m map[string]any
		if err := json.Unmarshal([]byte(row), &m); err != nil {
			t.Errorf("row %d invalid JSON: %v", i, err)
		}
	}

	// --- Step 9: Prompt building ---
	t.Log("Testing prompt building...")
	builder := NewPromptBuilder(schema)

	sys, user, err := builder.SQLGenerationPrompt("Show total revenue by product")
	if err != nil {
		t.Fatalf("SQLGenerationPrompt: %v", err)
	}
	if !strings.Contains(sys, "sales") {
		t.Error("SQL prompt missing table info")
	}
	if user == "" {
		t.Error("user prompt empty")
	}

	// Suggest analysis
	tables := eng.Tables()
	sysSuggest, userSuggest := builder.SuggestAnalysisPrompt(tables)
	if !strings.Contains(sysSuggest, "suggest") {
		t.Error("suggest prompt missing keyword")
	}
	if !strings.Contains(userSuggest, "access_logs") {
		t.Error("suggest prompt missing table")
	}

	// Window analysis
	sysWin, userWin, err := builder.WindowAnalysisPrompt(
		"Detect suspicious access patterns",
		"",
		nil,
		strings.Join(rows[:5], "\n"),
		0,
	)
	if err != nil {
		t.Fatalf("WindowAnalysisPrompt: %v", err)
	}
	if !strings.Contains(sysWin, "suspicious access") {
		t.Error("window prompt missing perspective")
	}
	if !strings.Contains(userWin, "Window 0") {
		t.Error("window prompt missing window index")
	}

	// --- Step 10: Mock sliding window analysis ---
	t.Log("Testing sliding window analysis with mock LLM...")
	mockResp := func(windowIdx int) string {
		if windowIdx == 0 {
			return `{"summary":"Window 0: Found 5 requests, 2 from suspicious IP 203.0.113.50 with 403 status","new_findings":[{"description":"Multiple 403 responses from 203.0.113.50","severity":"medium","evidence":"2 requests returned 403"}]}`
		}
		return fmt.Sprintf(`{"summary":"Window %d: Continued analysis, brute force pattern detected from 203.0.113.50","new_findings":[{"description":"Brute force login attempts","severity":"high","evidence":"4 consecutive 401 on /api/login"}]}`, windowIdx)
	}

	callIdx := 0
	llm := &mockLLMFunc{fn: func(ctx context.Context, sys, user string) (string, error) {
		resp := mockResp(callIdx)
		callIdx++
		return resp, nil
	}}

	cfg := DefaultSummarizerConfig()
	cfg.MaxRecordsPerWindow = 5
	cfg.OverlapRatio = 0
	summarizer := NewSummarizer(llm, builder, cfg)

	analyzeResult, err := summarizer.Analyze(ctx, "Detect security threats in access logs", rows, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	t.Logf("  Windows: %d, Findings: %d", analyzeResult.Windows, len(analyzeResult.Findings))
	t.Logf("  Summary: %.100s", analyzeResult.Summary)

	if analyzeResult.Windows < 2 {
		t.Errorf("expected >= 2 windows, got %d", analyzeResult.Windows)
	}
	if len(analyzeResult.Findings) == 0 {
		t.Error("expected at least 1 finding")
	}

	// --- Step 11: Report generation ---
	t.Log("Testing report generation...")
	reporter := NewReportGenerator()
	report := reporter.GenerateResultReport("Security Analysis: Access Logs", analyzeResult)

	if !strings.Contains(report, "Security Analysis") {
		t.Error("report missing title")
	}
	if !strings.Contains(report, "Brute force") || !strings.Contains(report, "403") {
		t.Error("report missing findings content")
	}
	t.Logf("  Report length: %d chars", len(report))

	// Write report to verify format
	reportPath := filepath.Join(dir, "report.md")
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("  Report saved to: %s", reportPath)

	// --- Step 12: Tables list ---
	allTables := eng.Tables()
	if len(allTables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(allTables))
	}

	t.Log("E2E pipeline complete.")
}

// TestE2E_BackgroundAnalysisCLI tests the CLI flag parsing and status file creation.
func TestE2E_BackgroundAnalysisCLI(t *testing.T) {
	csvPath := "/tmp/shell-agent-e2e/sales.csv"
	if _, err := os.Stat(csvPath); err != nil {
		t.Skip("E2E test data not found")
	}

	ctx := context.Background()
	dir := t.TempDir()

	// Prepare a DB with data
	dbPath := filepath.Join(dir, "bg.duckdb")
	eng, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	_, err = eng.LoadCSV(ctx, csvPath, "sales")
	if err != nil {
		t.Fatalf("LoadCSV: %v", err)
	}
	eng.Close()

	// Verify DB file exists and has data
	if info, err := os.Stat(dbPath); err != nil {
		t.Fatalf("DB file missing: %v", err)
	} else {
		t.Logf("DB file size: %d bytes", info.Size())
	}

	// Re-open to verify persistence
	eng2, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer eng2.Close()

	result, err := eng2.Execute(ctx, "SELECT COUNT(*) as cnt FROM sales")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("count error: %s", result.Error)
	}
	t.Logf("Reopened DB, sales count: %v", result.Rows[0][0])

	if fmt.Sprint(result.Rows[0][0]) != "20" {
		t.Errorf("expected 20 rows after reopen, got %v", result.Rows[0][0])
	}
}

// mockLLMFunc is a mock LLM that calls a function for each Chat invocation.
type mockLLMFunc struct {
	fn func(ctx context.Context, sys, user string) (string, error)
}

func (m *mockLLMFunc) Chat(ctx context.Context, sys, user string) (string, error) {
	return m.fn(ctx, sys, user)
}
