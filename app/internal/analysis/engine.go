// Package analysis provides data analysis capabilities using DuckDB
// with natural language query support and sliding window summarization.
package analysis

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/marcboeker/go-duckdb"
	_ "github.com/marcboeker/go-duckdb"
)

// dangerousSQLKeywords matches SQL write / data-definition / data-movement
// operations using word boundaries, so whitespace variants like "DROP\tTABLE"
// or "DELETE\nFROM" are still detected. LOAD / INSTALL are included because
// DuckDB extensions can execute arbitrary native code.
var dangerousSQLKeywords = regexp.MustCompile(
	`(?i)\b(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|TRUNCATE|REPLACE|GRANT|REVOKE|ATTACH|DETACH|COPY|EXPORT|IMPORT|LOAD|INSTALL|PRAGMA|EXECUTE|VACUUM)\b`,
)

// validSQLPrefixes matches the allowed first keyword of a read-only query
// (using a word boundary so "SELECT\n..." is accepted).
var validSQLPrefixes = regexp.MustCompile(`(?i)^(SELECT|EXPLAIN|DESCRIBE|SHOW|WITH)\b`)

// Engine provides DuckDB-backed data analysis capabilities.
type Engine struct {
	db     *sql.DB
	dbPath string
	tables map[string]*TableMeta
}

// NewEngine creates an analysis engine with a DuckDB database at the given path.
// If dbPath is empty, an in-memory database is used.
func NewEngine(dbPath string) (*Engine, error) {
	dsn := ""
	if dbPath != "" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
		dsn = dbPath
	}

	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}

	return &Engine{
		db:     db,
		dbPath: dbPath,
		tables: make(map[string]*TableMeta),
	}, nil
}

// Close closes the database connection.
func (e *Engine) Close() error {
	return e.db.Close()
}

// DB returns the underlying sql.DB for direct access.
func (e *Engine) DB() *sql.DB {
	return e.db
}

// DBPath returns the database file path.
func (e *Engine) DBPath() string {
	return e.dbPath
}

// LoadCSV loads a CSV file into a named table.
func (e *Engine) LoadCSV(ctx context.Context, filePath, tableName string) (*TableMeta, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	query := fmt.Sprintf(
		`CREATE OR REPLACE TABLE %s AS SELECT * FROM read_csv('%s', auto_detect=true)`,
		sanitizeIdentifier(tableName), escapeSQLString(absPath),
	)
	if _, err := e.db.ExecContext(ctx, query); err != nil {
		return nil, fmt.Errorf("load csv: %w", err)
	}

	return e.refreshTableMeta(ctx, tableName, absPath)
}

// LoadJSON loads a JSON file into a named table.
func (e *Engine) LoadJSON(ctx context.Context, filePath, tableName string) (*TableMeta, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	query := fmt.Sprintf(
		`CREATE OR REPLACE TABLE %s AS SELECT * FROM read_json('%s', auto_detect=true)`,
		sanitizeIdentifier(tableName), escapeSQLString(absPath),
	)
	if _, err := e.db.ExecContext(ctx, query); err != nil {
		return nil, fmt.Errorf("load json: %w", err)
	}

	return e.refreshTableMeta(ctx, tableName, absPath)
}

// LoadJSONL loads a JSONL (newline-delimited JSON) file into a named table.
func (e *Engine) LoadJSONL(ctx context.Context, filePath, tableName string) (*TableMeta, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	query := fmt.Sprintf(
		`CREATE OR REPLACE TABLE %s AS SELECT * FROM read_json('%s', format='newline_delimited', auto_detect=true)`,
		sanitizeIdentifier(tableName), escapeSQLString(absPath),
	)
	if _, err := e.db.ExecContext(ctx, query); err != nil {
		return nil, fmt.Errorf("load jsonl: %w", err)
	}

	return e.refreshTableMeta(ctx, tableName, absPath)
}

// LoadFile auto-detects the file format and loads it.
func (e *Engine) LoadFile(ctx context.Context, filePath, tableName string) (*TableMeta, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".csv", ".tsv":
		return e.LoadCSV(ctx, filePath, tableName)
	case ".json":
		return e.LoadJSON(ctx, filePath, tableName)
	case ".jsonl", ".ndjson":
		return e.LoadJSONL(ctx, filePath, tableName)
	default:
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}
}

// Execute runs a SQL query and returns the result.
func (e *Engine) Execute(ctx context.Context, sqlStr string) (*QueryResult, error) {
	start := time.Now()

	rows, err := e.db.QueryContext(ctx, sqlStr)
	if err != nil {
		return &QueryResult{
			SQL:   sqlStr,
			Error: err.Error(),
		}, nil
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	var resultRows [][]any
	for rows.Next() {
		values := make([]any, len(columns))
		scanArgs := make([]any, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		for i, v := range values {
			values[i] = normalizeValue(v)
		}
		resultRows = append(resultRows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return &QueryResult{
		SQL:      sqlStr,
		Columns:  columns,
		Rows:     resultRows,
		RowCount: len(resultRows),
		Duration: time.Since(start),
	}, nil
}

// DryRun validates SQL syntax using EXPLAIN.
func (e *Engine) DryRun(ctx context.Context, sqlStr string) error {
	_, err := e.db.ExecContext(ctx, "EXPLAIN "+sqlStr)
	return err
}

// LoadSchema returns the schema of all loaded tables as a formatted string for LLM context.
func (e *Engine) LoadSchema(ctx context.Context) (string, error) {
	rows, err := e.db.QueryContext(ctx,
		"SELECT table_name, column_name, data_type FROM information_schema.columns ORDER BY table_name, ordinal_position")
	if err != nil {
		return "", fmt.Errorf("query schema: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	currentTable := ""
	for rows.Next() {
		var table, column, dataType string
		if err := rows.Scan(&table, &column, &dataType); err != nil {
			return "", err
		}
		if table != currentTable {
			if currentTable != "" {
				sb.WriteString("\n")
			}
			fmt.Fprintf(&sb, "TABLE %s:", table)
			// Append description if available
			if meta, ok := e.tables[table]; ok && meta.Description != "" {
				fmt.Fprintf(&sb, " -- %s", meta.Description)
			}
			sb.WriteString("\n")
			currentTable = table
		}
		desc := ""
		if meta, ok := e.tables[table]; ok {
			for _, col := range meta.Columns {
				if col.Name == column && col.Description != "" {
					desc = " -- " + col.Description
					break
				}
			}
		}
		fmt.Fprintf(&sb, "  %s %s%s\n", column, dataType, desc)
	}
	return sb.String(), rows.Err()
}

// SetTableDescription sets a description for a table.
func (e *Engine) SetTableDescription(tableName, description string) {
	if meta, ok := e.tables[tableName]; ok {
		meta.Description = description
	}
}

// SetColumnDescription sets a description for a column.
func (e *Engine) SetColumnDescription(tableName, columnName, description string) {
	meta, ok := e.tables[tableName]
	if !ok {
		return
	}
	for i := range meta.Columns {
		if meta.Columns[i].Name == columnName {
			meta.Columns[i].Description = description
			return
		}
	}
}

// Tables returns metadata for all loaded tables.
func (e *Engine) Tables() []TableMeta {
	result := make([]TableMeta, 0, len(e.tables))
	for _, m := range e.tables {
		result = append(result, *m)
	}
	return result
}

// TableMeta returns metadata for a specific table.
func (e *Engine) TableMetaByName(name string) (*TableMeta, bool) {
	m, ok := e.tables[name]
	return m, ok
}

// ExportTable exports a table to a file (CSV or JSON).
func (e *Engine) ExportTable(ctx context.Context, tableName, outputPath, format string) error {
	var query string
	switch strings.ToLower(format) {
	case "csv":
		query = fmt.Sprintf(`COPY %s TO '%s' (FORMAT CSV, HEADER)`,
			sanitizeIdentifier(tableName), escapeSQLString(outputPath))
	case "json":
		query = fmt.Sprintf(`COPY %s TO '%s' (FORMAT JSON)`,
			sanitizeIdentifier(tableName), escapeSQLString(outputPath))
	case "parquet":
		query = fmt.Sprintf(`COPY %s TO '%s' (FORMAT PARQUET)`,
			sanitizeIdentifier(tableName), escapeSQLString(outputPath))
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
	_, err := e.db.ExecContext(ctx, query)
	return err
}

// ExportQuery exports query results to a file.
func (e *Engine) ExportQuery(ctx context.Context, sqlStr, outputPath, format string) error {
	var query string
	switch strings.ToLower(format) {
	case "csv":
		query = fmt.Sprintf(`COPY (%s) TO '%s' (FORMAT CSV, HEADER)`, sqlStr, escapeSQLString(outputPath))
	case "json":
		query = fmt.Sprintf(`COPY (%s) TO '%s' (FORMAT JSON)`, sqlStr, escapeSQLString(outputPath))
	case "parquet":
		query = fmt.Sprintf(`COPY (%s) TO '%s' (FORMAT PARQUET)`, sqlStr, escapeSQLString(outputPath))
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
	_, err := e.db.ExecContext(ctx, query)
	return err
}

// refreshTableMeta loads metadata for a table from the database.
func (e *Engine) refreshTableMeta(ctx context.Context, tableName, sourceFile string) (*TableMeta, error) {
	// Get columns
	colRows, err := e.db.QueryContext(ctx,
		"SELECT column_name, data_type, CASE WHEN is_nullable='YES' THEN true ELSE false END FROM information_schema.columns WHERE table_name = $1 ORDER BY ordinal_position",
		tableName)
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}
	defer colRows.Close()

	var columns []ColumnMeta
	for colRows.Next() {
		var col ColumnMeta
		if err := colRows.Scan(&col.Name, &col.Type, &col.Nullable); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		columns = append(columns, col)
	}

	// Get row count
	var rowCount int64
	if err := e.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s", sanitizeIdentifier(tableName)),
	).Scan(&rowCount); err != nil {
		return nil, fmt.Errorf("count rows: %w", err)
	}

	// Get sample data (up to 5 rows)
	sampleRows, err := e.db.QueryContext(ctx,
		fmt.Sprintf("SELECT * FROM %s LIMIT 5", sanitizeIdentifier(tableName)))
	if err != nil {
		return nil, fmt.Errorf("sample data: %w", err)
	}
	defer sampleRows.Close()

	sampleCols, _ := sampleRows.Columns()
	var sampleData []map[string]any
	for sampleRows.Next() {
		values := make([]any, len(sampleCols))
		scanArgs := make([]any, len(sampleCols))
		for i := range values {
			scanArgs[i] = &values[i]
		}
		if err := sampleRows.Scan(scanArgs...); err != nil {
			break
		}
		row := make(map[string]any)
		for i, col := range sampleCols {
			row[col] = normalizeValue(values[i])
		}
		sampleData = append(sampleData, row)
	}

	meta := &TableMeta{
		Name:       tableName,
		Columns:    columns,
		SampleData: sampleData,
		RowCount:   rowCount,
		LoadedAt:   time.Now(),
		SourceFile: sourceFile,
	}

	e.tables[tableName] = meta
	return meta, nil
}

// normalizeValue converts DuckDB-specific types to display-friendly values.
func normalizeValue(v any) any {
	switch d := v.(type) {
	case duckdb.Decimal:
		if d.Value == nil {
			return 0
		}
		f := new(big.Float).SetInt(d.Value)
		divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(d.Scale)), nil))
		result, _ := new(big.Float).Quo(f, divisor).Float64()
		return result
	case []byte:
		return string(d)
	default:
		return v
	}
}

// FormatResultSummary builds a context string for LLM conversation history.
func FormatResultSummary(result *QueryResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "SQL: %s\n", result.SQL)
	fmt.Fprintf(&sb, "Result: %d rows, columns: %s\n", result.RowCount, strings.Join(result.Columns, ", "))

	sampleCount := result.RowCount
	if sampleCount > 5 {
		sampleCount = 5
	}
	if sampleCount > 0 {
		sb.WriteString("Sample:\n")
		for i := 0; i < sampleCount; i++ {
			sb.WriteString("  ")
			for j, col := range result.Columns {
				if j > 0 {
					sb.WriteString(", ")
				}
				fmt.Fprintf(&sb, "%s=%v", col, result.Rows[i][j])
			}
			sb.WriteString("\n")
		}
		if result.RowCount > sampleCount {
			fmt.Fprintf(&sb, "  ... and %d more rows\n", result.RowCount-sampleCount)
		}
	}
	return sb.String()
}

// ResultToJSON converts a query result to a JSON string.
func ResultToJSON(result *QueryResult) string {
	rows := make([]map[string]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		m := make(map[string]any)
		for j, col := range result.Columns {
			if j < len(row) {
				m[col] = row[j]
			}
		}
		rows = append(rows, m)
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

// IsReadOnlySQL checks whether a SQL string is a read-only SELECT statement.
// Returns an error describing the violation if the SQL contains write operations.
//
// The check is intentionally conservative: dangerous keywords are matched with
// word boundaries (so "DROP\tTABLE" is caught the same as "DROP TABLE"), and
// a stripped copy of the query is used so literals / comments cannot influence
// the decision. Multiple statements (semicolons outside the final position)
// are rejected outright.
func IsReadOnlySQL(sqlStr string) error {
	trimmed := strings.TrimSpace(sqlStr)
	if trimmed == "" {
		return fmt.Errorf("empty query")
	}

	// Strip string literals and comments so keywords inside them don't affect
	// either the prefix check or the dangerous-keyword scan.
	stripped := stripSQLLiteralsAndComments(trimmed)
	strippedTrim := strings.TrimSpace(stripped)

	// Must start with a read-only keyword.
	if !validSQLPrefixes.MatchString(strippedTrim) {
		return fmt.Errorf("only SELECT queries are allowed")
	}

	// Check for dangerous keywords anywhere (including subqueries / CTEs).
	// Checked before the multi-statement rule so compound attacks like
	// "SELECT 1; DROP TABLE t" are reported against the dangerous keyword.
	if m := dangerousSQLKeywords.FindString(strippedTrim); m != "" {
		return fmt.Errorf("write operation %q is not allowed", strings.ToUpper(m))
	}

	// Reject multi-statement queries (a single trailing semicolon is OK).
	core := strings.TrimRight(strippedTrim, "; \t\r\n")
	if strings.Contains(core, ";") {
		return fmt.Errorf("multiple statements are not allowed")
	}

	return nil
}

// stripSQLLiteralsAndComments replaces string literals (both '...' and "...")
// and SQL comments (-- line, /* block */) with spaces. This lets IsReadOnlySQL
// scan the query for dangerous keywords without false positives from string
// content and without being bypassed by tricks like putting DROP inside a
// comment followed by a newline.
func stripSQLLiteralsAndComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	i := 0
	for i < len(s) {
		c := s[i]

		// Line comment: -- ... \n
		if c == '-' && i+1 < len(s) && s[i+1] == '-' {
			for i < len(s) && s[i] != '\n' {
				b.WriteByte(' ')
				i++
			}
			continue
		}

		// Block comment: /* ... */
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			b.WriteString("  ")
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				b.WriteByte(' ')
				i++
			}
			if i+1 < len(s) {
				b.WriteString("  ")
				i += 2
			}
			continue
		}

		// Single-quoted string literal; SQL escapes '' as a literal quote.
		if c == '\'' {
			b.WriteByte(' ')
			i++
			for i < len(s) {
				if s[i] == '\'' {
					if i+1 < len(s) && s[i+1] == '\'' {
						b.WriteString("  ")
						i += 2
						continue
					}
					b.WriteByte(' ')
					i++
					break
				}
				b.WriteByte(' ')
				i++
			}
			continue
		}

		// Double-quoted identifier / string; treat the same way for scanning.
		if c == '"' {
			b.WriteByte(' ')
			i++
			for i < len(s) && s[i] != '"' {
				b.WriteByte(' ')
				i++
			}
			if i < len(s) {
				b.WriteByte(' ')
				i++
			}
			continue
		}

		b.WriteByte(c)
		i++
	}
	return b.String()
}

// sanitizeIdentifier ensures a table/column name is safe for SQL.
func sanitizeIdentifier(name string) string {
	// Allow only alphanumeric and underscore
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		}
	}
	result := sb.String()
	if result == "" {
		return "unnamed"
	}
	return result
}

// escapeSQLString escapes single quotes in a SQL string literal.
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
