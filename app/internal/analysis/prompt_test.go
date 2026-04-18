package analysis

import (
	"fmt"
	"strings"
	"testing"
)

func TestSQLGenerationPrompt(t *testing.T) {
	builder := NewPromptBuilder("TABLE users:\n  id INTEGER\n  name VARCHAR\n")

	sys, user, err := builder.SQLGenerationPrompt("Show all users")
	if err != nil {
		t.Fatalf("SQLGenerationPrompt: %v", err)
	}

	if !strings.Contains(sys, "SQL query generator") {
		t.Error("system prompt should mention SQL generation")
	}
	if !strings.Contains(sys, "TABLE users") {
		t.Error("system prompt should contain schema")
	}
	if !strings.Contains(sys, "Only generate SELECT") {
		t.Error("system prompt should restrict to SELECT")
	}
	if user == "" {
		t.Error("user prompt should not be empty")
	}
}

func TestSQLGenerationPrompt_WithHistory(t *testing.T) {
	builder := NewPromptBuilder("TABLE t:\n  x INT\n")
	builder.AddHistory("user", "Show all records")
	builder.AddHistory("assistant", "SELECT * FROM t")

	_, user, err := builder.SQLGenerationPrompt("Filter by x > 10")
	if err != nil {
		t.Fatalf("SQLGenerationPrompt: %v", err)
	}

	if !strings.Contains(user, "Previous conversation") {
		t.Error("user prompt should contain history context")
	}
	if !strings.Contains(user, "Show all records") {
		t.Error("user prompt should contain prior question")
	}
}

func TestSQLGenerationPrompt_HistoryLimit(t *testing.T) {
	builder := NewPromptBuilder("TABLE t:\n  x INT\n")

	// Add 25 entries (exceeds 20 limit)
	for i := range 25 {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		builder.AddHistory(role, "entry")
	}

	if len(builder.history) != 20 {
		t.Errorf("history should be capped at 20, got %d", len(builder.history))
	}
}

func TestSQLFixPrompt(t *testing.T) {
	builder := NewPromptBuilder("TABLE t:\n  id INT\n")

	sys, user := builder.SQLFixPrompt("SELEC * FROM t", fmt.Errorf("syntax error"))
	if !strings.Contains(sys, "SQL query fixer") {
		t.Error("system prompt should mention fixing")
	}
	if !strings.Contains(user, "SELEC * FROM t") {
		t.Error("user prompt should contain broken SQL")
	}
	if !strings.Contains(user, "syntax error") {
		t.Error("user prompt should contain error message")
	}
}

func TestSummarizePrompt(t *testing.T) {
	result := &QueryResult{
		SQL:      "SELECT COUNT(*) FROM t",
		Columns:  []string{"count"},
		Rows:     [][]any{{42}},
		RowCount: 1,
	}

	builder := NewPromptBuilder("")
	sys, user := builder.SummarizePrompt(result)

	if !strings.Contains(sys, "data analyst") {
		t.Error("system prompt should mention data analyst")
	}
	if !strings.Contains(user, "SELECT COUNT") {
		t.Error("user prompt should contain SQL")
	}
}

func TestSuggestAnalysisPrompt(t *testing.T) {
	tables := []TableMeta{
		{
			Name:     "sales",
			RowCount: 1000,
			Columns:  []ColumnMeta{{Name: "date", Type: "DATE"}, {Name: "amount", Type: "DOUBLE"}},
		},
	}

	builder := NewPromptBuilder("")
	sys, user := builder.SuggestAnalysisPrompt(tables)

	if !strings.Contains(sys, "suggest") {
		t.Error("system prompt should mention suggestion")
	}
	if !strings.Contains(user, "sales") {
		t.Error("user prompt should contain table name")
	}
	if !strings.Contains(user, "1000 rows") {
		t.Error("user prompt should contain row count")
	}
}

func TestWindowAnalysisPrompt(t *testing.T) {
	builder := NewPromptBuilder("")

	sys, user, err := builder.WindowAnalysisPrompt(
		"Find anomalies in network traffic",
		"Previous analysis found normal patterns",
		[]Finding{{Description: "High traffic spike", Severity: "high"}},
		`{"src":"10.0.0.1","bytes":9999}`,
		3,
	)
	if err != nil {
		t.Fatalf("WindowAnalysisPrompt: %v", err)
	}

	if !strings.Contains(sys, "data analyst") {
		t.Error("system prompt should mention analyst role")
	}
	if !strings.Contains(sys, "Find anomalies") {
		t.Error("system prompt should contain perspective")
	}
	if !strings.Contains(user, "Previous Summary") {
		t.Error("user prompt should contain previous summary section")
	}
	if !strings.Contains(user, "High traffic spike") {
		t.Error("user prompt should contain existing findings")
	}
	if !strings.Contains(user, "Window 3") {
		t.Error("user prompt should contain window index")
	}
}

func TestFinalReportPrompt(t *testing.T) {
	findings := []Finding{
		{Description: "Found anomaly", Severity: "high", Evidence: "spike at 14:00"},
	}

	builder := NewPromptBuilder("")
	sys, user := builder.FinalReportPrompt("Overall summary here", findings)

	if !strings.Contains(sys, "final analysis report") {
		t.Error("system prompt should mention final report")
	}
	if !strings.Contains(user, "Overall summary here") {
		t.Error("user prompt should contain summary")
	}
	if !strings.Contains(user, "Found anomaly") {
		t.Error("user prompt should contain findings")
	}
}

func TestCleanSQL(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"SELECT 1", "SELECT 1"},
		{"```sql\nSELECT 1\n```", "SELECT 1"},
		{"```\nSELECT 1\n```", "SELECT 1"},
		{"  SELECT 1  ", "SELECT 1"},
	}
	for _, tt := range tests {
		got := CleanSQL(tt.input)
		if got != tt.want {
			t.Errorf("CleanSQL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestClearHistory(t *testing.T) {
	builder := NewPromptBuilder("")
	builder.AddHistory("user", "test")
	builder.ClearHistory()
	if len(builder.history) != 0 {
		t.Error("history should be empty after ClearHistory")
	}
}
