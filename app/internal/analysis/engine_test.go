package analysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewEngine_InMemory(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine in-memory: %v", err)
	}
	defer eng.Close()

	// Verify we can execute a simple query
	result, err := eng.Execute(context.Background(), "SELECT 1 AS n")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("query error: %s", result.Error)
	}
	if result.RowCount != 1 {
		t.Fatalf("expected 1 row, got %d", result.RowCount)
	}
}

func TestNewEngine_WithFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.duckdb")

	eng, err := NewEngine(dbPath)
	if err != nil {
		t.Fatalf("NewEngine file: %v", err)
	}
	defer eng.Close()

	if eng.DBPath() != dbPath {
		t.Fatalf("DBPath: got %q, want %q", eng.DBPath(), dbPath)
	}
}

func TestLoadCSV(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	// Create a test CSV
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "test.csv")
	if err := os.WriteFile(csvPath, []byte("name,age,score\nAlice,30,95.5\nBob,25,87.3\nCharlie,35,91.0\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	meta, err := eng.LoadCSV(context.Background(), csvPath, "users")
	if err != nil {
		t.Fatalf("LoadCSV: %v", err)
	}

	if meta.Name != "users" {
		t.Errorf("table name: got %q, want %q", meta.Name, "users")
	}
	if meta.RowCount != 3 {
		t.Errorf("row count: got %d, want 3", meta.RowCount)
	}
	if len(meta.Columns) != 3 {
		t.Errorf("columns: got %d, want 3", len(meta.Columns))
	}
	if len(meta.SampleData) != 3 {
		t.Errorf("sample data: got %d, want 3", len(meta.SampleData))
	}
}

func TestLoadJSON(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "test.json")
	if err := os.WriteFile(jsonPath, []byte(`[{"id":1,"value":"a"},{"id":2,"value":"b"}]`), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}

	meta, err := eng.LoadJSON(context.Background(), jsonPath, "items")
	if err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}

	if meta.RowCount != 2 {
		t.Errorf("row count: got %d, want 2", meta.RowCount)
	}
}

func TestLoadJSONL(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(jsonlPath, []byte("{\"id\":1,\"msg\":\"hello\"}\n{\"id\":2,\"msg\":\"world\"}\n{\"id\":3,\"msg\":\"test\"}\n"), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	meta, err := eng.LoadJSONL(context.Background(), jsonlPath, "logs")
	if err != nil {
		t.Fatalf("LoadJSONL: %v", err)
	}

	if meta.RowCount != 3 {
		t.Errorf("row count: got %d, want 3", meta.RowCount)
	}
}

func TestLoadFile_AutoDetect(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "auto.csv")
	if err := os.WriteFile(csvPath, []byte("x,y\n1,2\n3,4\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	meta, err := eng.LoadFile(context.Background(), csvPath, "auto")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if meta.RowCount != 2 {
		t.Errorf("row count: got %d, want 2", meta.RowCount)
	}
}

func TestLoadFile_UnsupportedFormat(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	_, err = eng.LoadFile(context.Background(), "test.xml", "data")
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestExecute(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	// Create a table
	_, err = eng.DB().Exec("CREATE TABLE t (id INT, name VARCHAR)")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = eng.DB().Exec("INSERT INTO t VALUES (1, 'a'), (2, 'b'), (3, 'c')")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := eng.Execute(context.Background(), "SELECT * FROM t ORDER BY id")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("query error: %s", result.Error)
	}
	if result.RowCount != 3 {
		t.Errorf("row count: got %d, want 3", result.RowCount)
	}
	if len(result.Columns) != 2 {
		t.Errorf("columns: got %d, want 2", len(result.Columns))
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestExecute_Error(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	result, err := eng.Execute(context.Background(), "SELECT * FROM nonexistent_table")
	if err != nil {
		t.Fatalf("Execute should return error in result, not as error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error in result for nonexistent table")
	}
}

func TestDryRun(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	_, _ = eng.DB().Exec("CREATE TABLE dr (id INT)")

	if err := eng.DryRun(context.Background(), "SELECT * FROM dr"); err != nil {
		t.Errorf("DryRun valid SQL: %v", err)
	}

	if err := eng.DryRun(context.Background(), "SELECT * FROM no_such_table"); err == nil {
		t.Error("DryRun should fail for nonexistent table")
	}
}

func TestLoadSchema(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	_, _ = eng.DB().Exec("CREATE TABLE products (id INTEGER, name VARCHAR, price DOUBLE)")

	schema, err := eng.LoadSchema(context.Background())
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}

	if schema == "" {
		t.Fatal("schema is empty")
	}
	if !contains(schema, "products") {
		t.Errorf("schema should contain table name 'products': %s", schema)
	}
	if !contains(schema, "price") {
		t.Errorf("schema should contain column 'price': %s", schema)
	}
}

func TestSetDescriptions(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "desc.csv")
	if err := os.WriteFile(csvPath, []byte("id,amount\n1,100\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	_, err = eng.LoadCSV(context.Background(), csvPath, "sales")
	if err != nil {
		t.Fatalf("LoadCSV: %v", err)
	}

	eng.SetTableDescription("sales", "Sales transactions")
	eng.SetColumnDescription("sales", "amount", "Transaction amount in JPY")

	schema, _ := eng.LoadSchema(context.Background())
	if !contains(schema, "Sales transactions") {
		t.Errorf("schema should contain table description: %s", schema)
	}
	if !contains(schema, "Transaction amount in JPY") {
		t.Errorf("schema should contain column description: %s", schema)
	}
}

func TestExportTable(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	_, _ = eng.DB().Exec("CREATE TABLE ex (id INT, val VARCHAR)")
	_, _ = eng.DB().Exec("INSERT INTO ex VALUES (1, 'a'), (2, 'b')")

	dir := t.TempDir()
	outPath := filepath.Join(dir, "export.csv")

	if err := eng.ExportTable(context.Background(), "ex", outPath, "csv"); err != nil {
		t.Fatalf("ExportTable: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if !contains(string(data), "id") || !contains(string(data), "val") {
		t.Errorf("exported CSV should contain headers: %s", data)
	}
}

func TestExportQuery(t *testing.T) {
	eng, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	_, _ = eng.DB().Exec("CREATE TABLE eq (n INT)")
	_, _ = eng.DB().Exec("INSERT INTO eq VALUES (10), (20), (30)")

	dir := t.TempDir()
	outPath := filepath.Join(dir, "query.json")

	if err := eng.ExportQuery(context.Background(), "SELECT * FROM eq WHERE n > 15", outPath, "json"); err != nil {
		t.Fatalf("ExportQuery: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if !contains(string(data), "20") || !contains(string(data), "30") {
		t.Errorf("export should contain 20 and 30: %s", data)
	}
}

func TestIsReadOnlySQL(t *testing.T) {
	// Valid read-only queries
	valid := []string{
		"SELECT * FROM t",
		"select count(*) from t",
		"  SELECT 1",
		"EXPLAIN SELECT * FROM t",
		"DESCRIBE t",
		"SHOW TABLES",
		"WITH cte AS (SELECT 1) SELECT * FROM cte",
	}
	for _, sql := range valid {
		if err := IsReadOnlySQL(sql); err != nil {
			t.Errorf("IsReadOnlySQL(%q) should be valid, got: %v", sql, err)
		}
	}

	// Invalid write queries
	invalid := []struct {
		sql  string
		want string
	}{
		{"INSERT INTO t VALUES (1)", "only SELECT"},
		{"DELETE FROM t", "only SELECT"},
		{"DROP TABLE t", "only SELECT"},
		{"UPDATE t SET x=1", "only SELECT"},
		{"CREATE TABLE t (id INT)", "only SELECT"},
		{"SELECT * FROM t; DROP TABLE t", "DROP"},
		{"TRUNCATE t", "only SELECT"},
		{"COPY t TO '/tmp/out.csv'", "only SELECT"},
	}
	for _, tt := range invalid {
		err := IsReadOnlySQL(tt.sql)
		if err == nil {
			t.Errorf("IsReadOnlySQL(%q) should fail", tt.sql)
		} else if !containsStr(err.Error(), tt.want) {
			t.Errorf("IsReadOnlySQL(%q) error = %v, want containing %q", tt.sql, err, tt.want)
		}
	}
}

// TestIsReadOnlySQL_WhitespaceBypass covers the whitespace-boundary bypass
// that existed before the regex-based rewrite. The old implementation matched
// dangerous keywords with a trailing literal space, so variants using a tab
// or newline between the keyword and the target silently passed.
func TestIsReadOnlySQL_WhitespaceBypass(t *testing.T) {
	cases := []struct {
		name string
		sql  string
	}{
		{"tab after DROP", "SELECT 1; DROP\tTABLE users"},
		{"newline after DELETE", "SELECT 1;\nDELETE\nFROM users"},
		{"carriage return after ATTACH", "SELECT 1; ATTACH\r'evil.db'"},
		{"trailing comment hides LOAD", "SELECT 1;\nLOAD\n'ext.so'"},
		{"INSTALL inside CTE", "WITH x AS (SELECT 1) INSTALL\thttpfs"},
		{"PRAGMA via whitespace", "SELECT 1; PRAGMA\tenable_profiling"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := IsReadOnlySQL(tc.sql); err == nil {
				t.Errorf("IsReadOnlySQL(%q) should be rejected", tc.sql)
			}
		})
	}
}

// TestIsReadOnlySQL_CommentInjection verifies that dangerous keywords hidden
// by comments (which are stripped before scanning) are still evaluated on the
// stripped remainder — a `-- DROP` comment should not mark the query as bad,
// but a real `DROP` hidden after a `/* ... */` block still is.
func TestIsReadOnlySQL_CommentInjection(t *testing.T) {
	// A keyword that only appears inside a line comment is not dangerous.
	if err := IsReadOnlySQL("SELECT 1 -- DROP TABLE t"); err != nil {
		t.Errorf("comment-only DROP should be allowed, got %v", err)
	}
	// A keyword that only appears inside a string literal is not dangerous.
	if err := IsReadOnlySQL("SELECT 'DROP TABLE t' AS msg"); err != nil {
		t.Errorf("string-literal DROP should be allowed, got %v", err)
	}
	// A real DROP after a block comment is still rejected.
	if err := IsReadOnlySQL("SELECT 1 /* safe */; DROP TABLE t"); err == nil {
		t.Errorf("real DROP after comment should be rejected")
	}
}

// TestIsReadOnlySQL_MultiStatement rejects any query with more than one
// statement, even when all statements are individually read-only. DuckDB
// implementations differ on multi-statement handling and we prefer refusal.
func TestIsReadOnlySQL_MultiStatement(t *testing.T) {
	if err := IsReadOnlySQL("SELECT 1; SELECT 2"); err == nil {
		t.Errorf("multi-SELECT should be rejected to keep attack surface minimal")
	}
	if err := IsReadOnlySQL("SELECT 1;"); err != nil {
		t.Errorf("single statement with trailing semicolon should be allowed, got %v", err)
	}
}

func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"users", "users"},
		{"my-table", "mytable"},
		{"table name!", "tablename"},
		{"", "unnamed"},
		{"123abc", "123abc"},
		{"valid_name_2", "valid_name_2"},
	}
	for _, tt := range tests {
		got := sanitizeIdentifier(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatResultSummary(t *testing.T) {
	result := &QueryResult{
		SQL:      "SELECT 1",
		Columns:  []string{"a", "b"},
		Rows:     [][]any{{1, "x"}, {2, "y"}},
		RowCount: 2,
	}
	s := FormatResultSummary(result)
	if !contains(s, "SELECT 1") {
		t.Errorf("summary missing SQL: %s", s)
	}
	if !contains(s, "2 rows") {
		t.Errorf("summary missing row count: %s", s)
	}
}

func TestResultToJSON(t *testing.T) {
	result := &QueryResult{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{1, "test"}},
	}
	j := ResultToJSON(result)
	if !contains(j, `"id"`) || !contains(j, `"name"`) {
		t.Errorf("JSON missing columns: %s", j)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
