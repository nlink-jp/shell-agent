package mcp

import (
	"sync"
	"testing"
)

// TestGuardian_Stop_Idempotent verifies that Stop() is safe to call more than
// once. Before the mutex-guarded rewrite, a second Stop() would trip
// "close of closed" on g.stdin or nil-dereference g.cmd.Process.
func TestGuardian_Stop_Idempotent(t *testing.T) {
	g := &Guardian{}
	// First Stop on an un-Started Guardian: must not panic, must not error.
	if err := g.Stop(); err != nil {
		t.Errorf("first Stop returned error: %v", err)
	}
	if !g.stopped {
		t.Error("stopped flag should be set after Stop")
	}
	// Second Stop is a no-op.
	if err := g.Stop(); err != nil {
		t.Errorf("second Stop returned error: %v", err)
	}
}

// TestGuardian_CallAfterStop confirms that a CallTool arriving after Stop()
// fails cleanly with a "stopped" error rather than racing on a closed pipe.
func TestGuardian_CallAfterStop(t *testing.T) {
	g := &Guardian{}
	_ = g.Stop()

	_, err := g.call("tools/list", nil)
	if err == nil {
		t.Fatal("expected error when calling a stopped guardian")
	}
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

// TestGuardian_ConcurrentStop hammers Stop() from many goroutines to
// confirm the mutex makes double-close impossible. A failure shows up as a
// panic from close-of-closed-channel or as a go-race report.
func TestGuardian_ConcurrentStop(t *testing.T) {
	g := &Guardian{}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.Stop()
		}()
	}
	wg.Wait()
	if !g.stopped {
		t.Error("stopped flag should be set")
	}
}
