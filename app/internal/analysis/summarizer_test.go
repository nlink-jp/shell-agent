package analysis

import (
	"context"
	"encoding/json"
	"testing"
)

// mockLLM implements LLMClient for testing.
type mockLLM struct {
	responses []string
	callCount int
}

func (m *mockLLM) Chat(_ context.Context, _, _ string) (string, error) {
	if m.callCount >= len(m.responses) {
		return `{"summary":"mock summary","new_findings":[]}`, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func TestAnalyze_EmptyData(t *testing.T) {
	llm := &mockLLM{}
	builder := NewPromptBuilder("")
	s := NewSummarizer(llm, builder, DefaultSummarizerConfig())

	result, err := s.Analyze(context.Background(), "test perspective", nil, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.Summary != "No data to analyze." {
		t.Errorf("expected empty data message, got: %s", result.Summary)
	}
}

func TestAnalyze_SingleWindow(t *testing.T) {
	llm := &mockLLM{
		responses: []string{
			`{"summary":"Found 3 records with normal values","new_findings":[{"description":"All values in range","severity":"info","evidence":"values 1-3"}]}`,
		},
	}
	builder := NewPromptBuilder("TABLE t:\n  id INT\n")
	s := NewSummarizer(llm, builder, DefaultSummarizerConfig())

	rows := []string{
		`{"id":1}`,
		`{"id":2}`,
		`{"id":3}`,
	}

	result, err := s.Analyze(context.Background(), "Check data quality", rows, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if result.Windows != 1 {
		t.Errorf("expected 1 window, got %d", result.Windows)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
}

func TestAnalyze_MultipleWindows(t *testing.T) {
	llm := &mockLLM{
		responses: []string{
			`{"summary":"Window 1 summary","new_findings":[{"description":"Finding 1","severity":"low","evidence":"ev1"}]}`,
			`{"summary":"Window 2 summary","new_findings":[{"description":"Finding 2","severity":"medium","evidence":"ev2"}]}`,
			`{"summary":"Final combined summary","new_findings":[]}`,
		},
	}
	builder := NewPromptBuilder("")
	cfg := DefaultSummarizerConfig()
	cfg.MaxRecordsPerWindow = 2
	cfg.OverlapRatio = 0
	s := NewSummarizer(llm, builder, cfg)

	rows := []string{
		`{"id":1}`,
		`{"id":2}`,
		`{"id":3}`,
		`{"id":4}`,
	}

	result, err := s.Analyze(context.Background(), "Analyze data", rows, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if result.Windows < 2 {
		t.Errorf("expected >= 2 windows, got %d", result.Windows)
	}
	if len(result.Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(result.Findings))
	}
}

func TestAnalyze_StatusCallback(t *testing.T) {
	llm := &mockLLM{
		responses: []string{
			`{"summary":"s","new_findings":[]}`,
		},
	}
	builder := NewPromptBuilder("")
	s := NewSummarizer(llm, builder, DefaultSummarizerConfig())

	var statuses []string
	result, err := s.Analyze(context.Background(), "test", []string{`{"x":1}`}, func(idx, total int, status string) {
		statuses = append(statuses, status)
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	_ = result

	if len(statuses) == 0 {
		t.Error("expected at least one status callback")
	}
}

func TestAnalyze_Cancellation(t *testing.T) {
	llm := &mockLLM{}
	builder := NewPromptBuilder("")
	s := NewSummarizer(llm, builder, DefaultSummarizerConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := s.Analyze(ctx, "test", []string{`{"x":1}`}, nil)
	if err == nil {
		t.Error("expected cancellation error")
	}
}

func TestEvictFindings(t *testing.T) {
	findings := []Finding{
		{Description: "f1", Severity: "info"},
		{Description: "f2", Severity: "low"},
		{Description: "f3", Severity: "high"},
		{Description: "f4", Severity: "medium"},
		{Description: "f5", Severity: "info"},
	}

	result := evictFindings(findings, 3)
	if len(result) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(result))
	}

	// High and medium should be kept
	hasHigh, hasMedium := false, false
	for _, f := range result {
		if f.Severity == "high" {
			hasHigh = true
		}
		if f.Severity == "medium" {
			hasMedium = true
		}
	}
	if !hasHigh || !hasMedium {
		t.Error("high and medium severity should be preserved")
	}
}

func TestEvictFindings_NoEvictionNeeded(t *testing.T) {
	findings := []Finding{
		{Description: "f1", Severity: "info"},
		{Description: "f2", Severity: "low"},
	}
	result := evictFindings(findings, 10)
	if len(result) != 2 {
		t.Errorf("should not evict when under limit: got %d", len(result))
	}
}

func TestValidateSeverity(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"info", "info"},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"critical", "critical"},
		{"HIGH", "high"},
		{"unknown", "info"},
		{"", "info"},
	}
	for _, tt := range tests {
		got := validateSeverity(tt.input)
		if got != tt.want {
			t.Errorf("validateSeverity(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	if EstimateTokens("") != 0 {
		t.Error("empty string should be 0 tokens")
	}

	// Short English text
	tokens := EstimateTokens("Hello world")
	if tokens <= 0 {
		t.Error("expected positive token count for English text")
	}

	// JSON-heavy text should use char-based estimation
	jsonStr := `{"key":"value","nested":{"array":[1,2,3]}}`
	jsonTokens := EstimateTokens(jsonStr)
	if jsonTokens < len(jsonStr)/4 {
		t.Errorf("JSON tokens should be at least char-based: got %d, expected >= %d", jsonTokens, len(jsonStr)/4)
	}
}

func TestResultToRows(t *testing.T) {
	result := &QueryResult{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{1, "Alice"}, {2, "Bob"}},
	}

	rows := ResultToRows(result)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(rows[0]), &m); err != nil {
		t.Fatalf("unmarshal row: %v", err)
	}
	if m["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", m["name"])
	}
}
