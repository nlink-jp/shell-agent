// Package analysis provides data analysis capabilities using DuckDB
// with natural language query support and sliding window summarization.
package analysis

import "time"

// TableMeta holds metadata about a loaded table.
type TableMeta struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Columns     []ColumnMeta `json:"columns"`
	SampleData  []map[string]any `json:"sample_data,omitempty"`
	RowCount    int64        `json:"row_count"`
	LoadedAt    time.Time    `json:"loaded_at"`
	SourceFile  string       `json:"source_file,omitempty"`
	ObjectID    string       `json:"object_id,omitempty"` // objstore ID of source data
}

// ColumnMeta holds metadata about a table column.
type ColumnMeta struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Nullable    bool   `json:"nullable"`
}

// QueryResult holds the result of a SQL execution.
type QueryResult struct {
	SQL       string           `json:"sql"`
	Columns   []string         `json:"columns"`
	Rows      [][]any          `json:"rows"`
	RowCount  int              `json:"row_count"`
	Duration  time.Duration    `json:"duration"`
	Error     string           `json:"error,omitempty"`
	Summary   string           `json:"summary,omitempty"`
	ObjectID  string           `json:"object_id,omitempty"` // objstore ID if exported
}

// Finding represents a discovery from analysis.
type Finding struct {
	Description string    `json:"description"`
	Severity    string    `json:"severity"` // info, low, medium, high, critical
	Evidence    string    `json:"evidence"`
	QuerySQL    string    `json:"query_sql,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// AnalysisState holds the persistent state of an analysis session.
type AnalysisState struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Tables      []TableMeta  `json:"tables"`
	Findings    []Finding    `json:"findings"`
	Summary     string       `json:"summary"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// MemoryBudget defines token allocation for analysis context.
type MemoryBudget struct {
	Total       int `json:"total"`
	Schema      int `json:"schema"`
	Summary     int `json:"summary"`
	Findings    int `json:"findings"`
	RawData     int `json:"raw_data"`
	Response    int `json:"response"`
}

// DefaultMemoryBudget returns a reasonable budget for a 128K context model.
func DefaultMemoryBudget() MemoryBudget {
	return MemoryBudget{
		Total:    65536,
		Schema:   4000,
		Summary:  15000,
		Findings: 20000,
		RawData:  20000,
		Response: 6536,
	}
}
