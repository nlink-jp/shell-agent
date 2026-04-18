package analysis

import (
	"fmt"
	"strings"
	"time"
)

// ReportGenerator creates markdown reports from analysis results.
type ReportGenerator struct{}

// NewReportGenerator creates a report generator.
func NewReportGenerator() *ReportGenerator {
	return &ReportGenerator{}
}

// GenerateReport creates a markdown report from an analysis state.
func (g *ReportGenerator) GenerateReport(state *AnalysisState) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", state.Title))
	sb.WriteString(fmt.Sprintf("> Generated: %s\n\n", state.UpdatedAt.Format("2006-01-02 15:04:05")))

	// Summary section
	sb.WriteString("## Summary\n\n")
	if state.Summary != "" {
		sb.WriteString(state.Summary)
	} else {
		sb.WriteString("No summary available.")
	}
	sb.WriteString("\n\n")

	// Data sources
	if len(state.Tables) > 0 {
		sb.WriteString("## Data Sources\n\n")
		sb.WriteString("| Table | Rows | Source | Loaded |\n")
		sb.WriteString("|-------|------|--------|--------|\n")
		for _, t := range state.Tables {
			source := t.SourceFile
			if source == "" {
				source = "-"
			}
			sb.WriteString(fmt.Sprintf("| %s | %d | %s | %s |\n",
				t.Name, t.RowCount, source, t.LoadedAt.Format("15:04:05")))
		}
		sb.WriteString("\n")
	}

	// Findings by severity
	if len(state.Findings) > 0 {
		sb.WriteString("## Findings\n\n")

		// Group by severity
		severityOrder := []string{"critical", "high", "medium", "low", "info"}
		grouped := make(map[string][]Finding)
		for _, f := range state.Findings {
			grouped[f.Severity] = append(grouped[f.Severity], f)
		}

		for _, sev := range severityOrder {
			items, ok := grouped[sev]
			if !ok {
				continue
			}
			sb.WriteString(fmt.Sprintf("### %s (%d)\n\n", severityLabel(sev), len(items)))
			for _, f := range items {
				sb.WriteString(fmt.Sprintf("- **%s**\n", f.Description))
				if f.Evidence != "" {
					sb.WriteString(fmt.Sprintf("  - Evidence: %s\n", f.Evidence))
				}
				if f.QuerySQL != "" {
					sb.WriteString(fmt.Sprintf("  - SQL: `%s`\n", f.QuerySQL))
				}
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// GenerateResultReport creates a simple report from a single analysis result.
func (g *ReportGenerator) GenerateResultReport(title string, result *AnalyzeResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	sb.WriteString(fmt.Sprintf("> Generated: %s | Windows: %d | Duration: %s\n\n",
		time.Now().Format("2006-01-02 15:04:05"),
		result.Windows,
		result.Duration.Round(time.Second)))

	sb.WriteString("## Summary\n\n")
	sb.WriteString(result.Summary)
	sb.WriteString("\n\n")

	if len(result.Findings) > 0 {
		sb.WriteString("## Findings\n\n")

		severityOrder := []string{"critical", "high", "medium", "low", "info"}
		grouped := make(map[string][]Finding)
		for _, f := range result.Findings {
			grouped[f.Severity] = append(grouped[f.Severity], f)
		}

		for _, sev := range severityOrder {
			items, ok := grouped[sev]
			if !ok {
				continue
			}
			sb.WriteString(fmt.Sprintf("### %s (%d)\n\n", severityLabel(sev), len(items)))
			for _, f := range items {
				sb.WriteString(fmt.Sprintf("- **%s**\n", f.Description))
				if f.Evidence != "" {
					sb.WriteString(fmt.Sprintf("  - Evidence: %s\n", f.Evidence))
				}
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func severityLabel(s string) string {
	switch s {
	case "critical":
		return "Critical"
	case "high":
		return "High"
	case "medium":
		return "Medium"
	case "low":
		return "Low"
	case "info":
		return "Info"
	default:
		if s == "" {
			return "Unknown"
		}
		return strings.ToUpper(s[:1]) + s[1:]
	}
}
