package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestIsValidAnalysisJobID exercises the validation added to guard
// analysisStatusTool / analysisResultTool against LLM-supplied job IDs like
// "../../etc" that would otherwise escape analysisDir via filepath.Join.
func TestIsValidAnalysisJobID(t *testing.T) {
	valid := []string{"job-1", "job-12345", "job-1735689600"}
	for _, id := range valid {
		if !isValidAnalysisJobID(id) {
			t.Errorf("isValidAnalysisJobID(%q) = false, want true", id)
		}
	}

	invalid := []string{
		"",
		"job-",
		"../job-1",
		"job-1/../../etc",
		"job-1/../job-2",
		"job-abc",
		"../../etc/passwd",
		"/absolute/job-1",
		"session-1",
		"job-1.bak",
	}
	for _, id := range invalid {
		if isValidAnalysisJobID(id) {
			t.Errorf("isValidAnalysisJobID(%q) = true, want false", id)
		}
	}
}

// TestAnalysisStatusTool_PathTraversal confirms that a malicious job_id is
// rejected at the tool boundary and never reaches os.ReadFile. The attack
// shape — a dotted prefix followed by an arbitrary path — matches what an
// LLM coerced by prompt injection could emit.
func TestAnalysisStatusTool_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	a := &App{analysisDir: filepath.Join(dir, "analysis")}
	if err := os.MkdirAll(a.analysisDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Plant a "secret" file outside analysisDir to prove it stays unreadable.
	secretPath := filepath.Join(dir, "secret.json")
	if err := os.WriteFile(secretPath, []byte(`{"state":"PWNED"}`), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	malicious, _ := json.Marshal(map[string]string{"job_id": "../secret"})
	out := a.analysisStatusTool(string(malicious))
	if !strings.Contains(out, "invalid job_id") {
		t.Errorf("expected rejection of traversal job_id, got: %s", out)
	}
	if strings.Contains(out, "PWNED") {
		t.Errorf("traversal succeeded and leaked secret content: %s", out)
	}
}

func TestAnalysisResultTool_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	a := &App{analysisDir: filepath.Join(dir, "analysis")}
	if err := os.MkdirAll(a.analysisDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	malicious, _ := json.Marshal(map[string]string{"job_id": "../../etc/passwd"})
	out := a.analysisResultTool(string(malicious))
	if !strings.Contains(out, "invalid job_id") {
		t.Errorf("expected rejection of traversal job_id, got: %s", out)
	}
}

// TestExtractImageFromResult_PathTraversal verifies the Critical #1 fix:
// a tool output containing {"path":"/etc/passwd"} must not be honored.
// The helper now delegates to fileToDataURL, which enforces an allowlist
// of base directories (/tmp/shell-agent-images, config dir) and objstore IDs.
func TestExtractImageFromResult_PathTraversal(t *testing.T) {
	a := &App{}

	// Absolute path outside the allowlist — must be refused.
	result := `{"path":"/etc/passwd"}`
	if got := a.extractImageFromResult(result); got != "" {
		t.Errorf("extractImageFromResult should reject /etc/passwd, got: %q", got)
	}

	// Relative traversal inside a legitimate-looking key — must also be refused.
	result = `{"filename":"../../../../etc/hosts"}`
	if got := a.extractImageFromResult(result); got != "" {
		t.Errorf("extractImageFromResult should reject traversal, got: %q", got)
	}

	// Non-JSON input should return empty without error.
	if got := a.extractImageFromResult("not json at all"); got != "" {
		t.Errorf("extractImageFromResult non-JSON should return empty, got: %q", got)
	}
}

// TestTokenStatsConcurrency exercises the statsMu mutex by hammering the
// writer and reader paths from many goroutines. Without the mutex, running
// under `go test -race` would report a data race on tokenStats fields.
func TestTokenStatsConcurrency(t *testing.T) {
	a := &App{}
	const workers = 16
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				a.addTokenUsage(1, 2)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = a.snapshotTokenStats()
				_, _ = a.lastTokenUsage()
			}
		}()
	}
	wg.Wait()

	got := a.snapshotTokenStats()
	wantIn := workers * iterations
	wantOut := workers * iterations * 2
	if got.TotalInput != wantIn {
		t.Errorf("TotalInput = %d, want %d", got.TotalInput, wantIn)
	}
	if got.TotalOutput != wantOut {
		t.Errorf("TotalOutput = %d, want %d", got.TotalOutput, wantOut)
	}
}
