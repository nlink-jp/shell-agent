package analysis

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateReport(t *testing.T) {
	gen := NewReportGenerator()
	state := &AnalysisState{
		Title:   "Test Analysis",
		Summary: "Found interesting patterns in the data.",
		Tables: []TableMeta{
			{Name: "logs", RowCount: 1000, LoadedAt: time.Now()},
		},
		Findings: []Finding{
			{Description: "High latency spike", Severity: "high", Evidence: "p99 > 500ms at 14:00"},
			{Description: "Normal traffic pattern", Severity: "info", Evidence: "avg 100 req/s"},
		},
		UpdatedAt: time.Now(),
	}

	report := gen.GenerateReport(state)

	if !strings.Contains(report, "# Test Analysis") {
		t.Error("report should contain title")
	}
	if !strings.Contains(report, "Found interesting patterns") {
		t.Error("report should contain summary")
	}
	if !strings.Contains(report, "logs") {
		t.Error("report should contain table name")
	}
	if !strings.Contains(report, "High latency spike") {
		t.Error("report should contain findings")
	}
	if !strings.Contains(report, "### High (1)") {
		t.Error("report should group findings by severity")
	}
	if !strings.Contains(report, "### Info (1)") {
		t.Error("report should include info findings")
	}
}

func TestGenerateReport_Empty(t *testing.T) {
	gen := NewReportGenerator()
	state := &AnalysisState{
		Title:     "Empty Analysis",
		UpdatedAt: time.Now(),
	}

	report := gen.GenerateReport(state)
	if !strings.Contains(report, "No summary available") {
		t.Error("empty report should indicate no summary")
	}
}

func TestGenerateResultReport(t *testing.T) {
	gen := NewReportGenerator()
	result := &AnalyzeResult{
		Summary: "Analysis complete with key findings.",
		Findings: []Finding{
			{Description: "Critical issue", Severity: "critical", Evidence: "data corruption detected"},
			{Description: "Minor observation", Severity: "low", Evidence: "slight variance"},
		},
		Windows:  5,
		Duration: 30 * time.Second,
	}

	report := gen.GenerateResultReport("Network Analysis", result)

	if !strings.Contains(report, "# Network Analysis") {
		t.Error("report should contain title")
	}
	if !strings.Contains(report, "Windows: 5") {
		t.Error("report should contain window count")
	}
	if !strings.Contains(report, "### Critical (1)") {
		t.Error("report should group by severity")
	}
	if !strings.Contains(report, "### Low (1)") {
		t.Error("report should include low findings")
	}
}

func TestSeverityLabel(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"critical", "Critical"},
		{"high", "High"},
		{"medium", "Medium"},
		{"low", "Low"},
		{"info", "Info"},
	}
	for _, tt := range tests {
		got := severityLabel(tt.input)
		if got != tt.want {
			t.Errorf("severityLabel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
