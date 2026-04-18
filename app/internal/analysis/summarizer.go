package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nlink-jp/nlk/jsonfix"
)

// LLMClient is the interface for LLM API calls used by the summarizer.
type LLMClient interface {
	Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// SummarizerConfig holds configuration for the sliding window summarizer.
type SummarizerConfig struct {
	MaxRecordsPerWindow int // max records per window (quality guard)
	OverlapRatio        float64
	ContextLimit        int // total token budget
	MaxSummaryTokens    int
	MaxFindingsTokens   int
	MinRawDataTokens    int
	SystemReserve       int
	ResponseReserve     int
	MaxFindings         int // max accumulated findings before eviction
}

// DefaultSummarizerConfig returns reasonable defaults for local LLM analysis.
func DefaultSummarizerConfig() SummarizerConfig {
	return SummarizerConfig{
		MaxRecordsPerWindow: 100,
		OverlapRatio:        0.1,
		ContextLimit:        65536,
		MaxSummaryTokens:    10000,
		MaxFindingsTokens:   15000,
		MinRawDataTokens:    8000,
		SystemReserve:       2000,
		ResponseReserve:     4000,
		MaxFindings:         50,
	}
}

// Summarizer implements sliding window analysis over query results.
type Summarizer struct {
	llm     LLMClient
	builder *PromptBuilder
	cfg     SummarizerConfig
}

// NewSummarizer creates a sliding window summarizer.
func NewSummarizer(llm LLMClient, builder *PromptBuilder, cfg SummarizerConfig) *Summarizer {
	return &Summarizer{llm: llm, builder: builder, cfg: cfg}
}

// WindowResponse is the expected JSON response from the LLM for each window.
type WindowResponse struct {
	Summary     string        `json:"summary"`
	NewFindings []FindingJSON `json:"new_findings"`
}

// FindingJSON is the JSON shape of a finding from the LLM.
type FindingJSON struct {
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Evidence    string `json:"evidence"`
}

// AnalyzeResult holds the final result of a full analysis run.
type AnalyzeResult struct {
	Summary  string    `json:"summary"`
	Findings []Finding `json:"findings"`
	Windows  int       `json:"windows"`
	Duration time.Duration
}

// StatusCallback is called with progress updates during analysis.
type StatusCallback func(windowIndex, totalWindows int, status string)

// Analyze runs sliding window analysis over the given data rows.
// The data is provided as JSON-encoded rows (one string per row).
func (s *Summarizer) Analyze(
	ctx context.Context,
	perspective string,
	rows []string,
	onStatus StatusCallback,
) (*AnalyzeResult, error) {
	start := time.Now()

	if len(rows) == 0 {
		return &AnalyzeResult{Summary: "No data to analyze."}, nil
	}

	// Estimate total windows
	recordsPerWindow := s.cfg.MaxRecordsPerWindow
	if recordsPerWindow <= 0 {
		recordsPerWindow = 100
	}
	overlapCount := int(float64(recordsPerWindow) * s.cfg.OverlapRatio)
	step := recordsPerWindow - overlapCount
	if step <= 0 {
		step = 1
	}
	totalWindows := (len(rows) + step - 1) / step

	var summary string
	var findings []Finding
	windowIndex := 0
	offset := 0

	for offset < len(rows) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Determine window size, respecting token budget
		end := offset + recordsPerWindow
		if end > len(rows) {
			end = len(rows)
		}
		windowRows := rows[offset:end]

		if onStatus != nil {
			onStatus(windowIndex, totalWindows, fmt.Sprintf("Analyzing window %d/%d (%d records)", windowIndex+1, totalWindows, len(windowRows)))
		}

		// Build data chunk
		dataChunk := strings.Join(windowRows, "\n")

		// Truncate if data chunk exceeds raw data budget
		maxChars := s.cfg.MinRawDataTokens * 4 // rough: 4 chars per token
		if len(dataChunk) > maxChars {
			dataChunk = dataChunk[:maxChars] + "\n... (truncated)"
		}

		// Build and send prompt
		sysPrompt, userPrompt, err := s.builder.WindowAnalysisPrompt(
			perspective, summary, findings, dataChunk, windowIndex,
		)
		if err != nil {
			return nil, fmt.Errorf("build window prompt: %w", err)
		}

		response, err := s.llm.Chat(ctx, sysPrompt, userPrompt)
		if err != nil {
			return nil, fmt.Errorf("llm chat (window %d): %w", windowIndex, err)
		}

		// Parse response
		windowResp, err := parseWindowResponse(response)
		if err != nil {
			// Try to continue with previous state on parse failure
			windowIndex++
			offset += step
			continue
		}

		// Update running summary
		if windowResp.Summary != "" {
			summary = windowResp.Summary
		}

		// Append new findings
		now := time.Now()
		for _, f := range windowResp.NewFindings {
			findings = append(findings, Finding{
				Description: f.Description,
				Severity:    validateSeverity(f.Severity),
				Evidence:    f.Evidence,
				Timestamp:   now,
			})
		}

		// Evict low-priority findings if over limit
		findings = evictFindings(findings, s.cfg.MaxFindings)

		windowIndex++
		offset += step
	}

	// Generate final report if we had multiple windows
	if windowIndex > 1 {
		if onStatus != nil {
			onStatus(windowIndex, totalWindows, "Generating final report")
		}
		finalSummary, err := s.generateFinalReport(ctx, perspective, summary, findings)
		if err == nil && finalSummary != "" {
			summary = finalSummary
		}
	}

	return &AnalyzeResult{
		Summary:  summary,
		Findings: findings,
		Windows:  windowIndex,
		Duration: time.Since(start),
	}, nil
}

// SummarizeResult generates a concise summary of a query result.
func (s *Summarizer) SummarizeResult(ctx context.Context, result *QueryResult) (string, error) {
	sys, user := s.builder.SummarizePrompt(result)
	return s.llm.Chat(ctx, sys, user)
}

// SuggestAnalysis suggests analysis perspectives based on table metadata.
func (s *Summarizer) SuggestAnalysis(ctx context.Context, tables []TableMeta) (string, error) {
	sys, user := s.builder.SuggestAnalysisPrompt(tables)
	return s.llm.Chat(ctx, sys, user)
}

func (s *Summarizer) generateFinalReport(ctx context.Context, perspective, summary string, findings []Finding) (string, error) {
	sys, user := s.builder.FinalReportPrompt(summary, findings)
	response, err := s.llm.Chat(ctx, sys, user)
	if err != nil {
		return "", err
	}

	var resp WindowResponse
	if err := jsonfix.ExtractTo(response, &resp); err != nil {
		// If JSON parse fails, use the raw response as summary
		return response, nil
	}
	return resp.Summary, nil
}

func parseWindowResponse(text string) (*WindowResponse, error) {
	var resp WindowResponse
	if err := jsonfix.ExtractTo(text, &resp); err != nil {
		return nil, fmt.Errorf("parse window response: %w (raw: %.200s)", err, text)
	}
	return &resp, nil
}

func validateSeverity(s string) string {
	switch strings.ToLower(s) {
	case "info", "low", "medium", "high", "critical":
		return strings.ToLower(s)
	default:
		return "info"
	}
}

func evictFindings(findings []Finding, maxFindings int) []Finding {
	if maxFindings <= 0 || len(findings) <= maxFindings {
		return findings
	}

	// Separate by priority
	var high, other []Finding
	for _, f := range findings {
		switch f.Severity {
		case "high", "critical", "medium":
			high = append(high, f)
		default:
			other = append(other, f)
		}
	}

	// Keep all high/critical/medium, evict info/low (FIFO)
	remaining := maxFindings - len(high)
	if remaining < 0 {
		// Even high-priority exceeds limit; keep most recent
		return high[len(high)-maxFindings:]
	}
	if remaining < len(other) {
		other = other[len(other)-remaining:]
	}

	return append(high, other...)
}

// EstimateTokens provides a rough token count for text.
// Uses dual estimation: word-based and char-based, takes the max.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	// Char-based: ~4 chars per token (good for JSON/structured data)
	charBased := len(text) / 4

	// Word-based with CJK awareness
	wordBased := 0
	words := strings.Fields(text)
	for _, w := range words {
		cjk := 0
		ascii := 0
		for _, r := range w {
			if r >= 0x3000 && r <= 0x9FFF || r >= 0xF900 && r <= 0xFAFF {
				cjk++
			} else {
				ascii++
			}
		}
		if cjk > 0 {
			wordBased += cjk * 2
		}
		if ascii > 0 {
			wordBased++ // count as one word-token
		}
	}

	if charBased > wordBased {
		return charBased
	}
	return wordBased
}

// ResultToRows converts a QueryResult into individual JSON row strings
// suitable for sliding window analysis.
func ResultToRows(result *QueryResult) []string {
	rows := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		m := make(map[string]any)
		for j, col := range result.Columns {
			if j < len(row) {
				m[col] = row[j]
			}
		}
		data, err := json.Marshal(m)
		if err != nil {
			continue
		}
		rows = append(rows, string(data))
	}
	return rows
}
