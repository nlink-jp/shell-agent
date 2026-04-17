package memory

import (
	"testing"
	"time"
)

func TestStoreNewAndLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().Truncate(time.Second)
	session := &Session{
		ID:        "test-session-1",
		Title:     "Test Session",
		CreatedAt: now,
		UpdatedAt: now,
		Records: []Record{
			{
				Timestamp: now,
				Role:      "user",
				Content:   "Hello",
				Tier:      TierHot,
			},
			{
				Timestamp: now.Add(time.Minute),
				Role:      "assistant",
				Content:   "Hi there!",
				Tier:      TierHot,
			},
		},
	}

	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load("test-session-1")
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Title != "Test Session" {
		t.Errorf("unexpected title: %s", loaded.Title)
	}
	if len(loaded.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(loaded.Records))
	}
	if loaded.Records[0].Content != "Hello" {
		t.Errorf("unexpected content: %s", loaded.Records[0].Content)
	}
	if loaded.Records[1].Tier != TierHot {
		t.Errorf("unexpected tier: %s", loaded.Records[1].Tier)
	}
}

func TestStoreList(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().Truncate(time.Second)
	for _, id := range []string{"s1", "s2", "s3"} {
		session := &Session{
			ID:        id,
			Title:     "Session " + id,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := store.Save(session); err != nil {
			t.Fatal(err)
		}
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatal(err)
	}

	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}
}

func TestStoreLoadNotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Load("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestEstimateTokens(t *testing.T) {
	// English text: ~4 chars per token
	eng := "Hello world, this is a test message for token estimation."
	tokens := EstimateTokens(eng)
	if tokens < 10 || tokens > 30 {
		t.Errorf("unexpected token count for English: %d", tokens)
	}

	// Japanese text: ~2 chars per token
	jpn := "これはトークン推定のテストメッセージです。日本語のテキストです��"
	tokensJP := EstimateTokens(jpn)
	if tokensJP < 10 {
		t.Errorf("unexpected token count for Japanese: %d", tokensJP)
	}
}

func TestHotTokenCount(t *testing.T) {
	session := &Session{
		Records: []Record{
			{Role: "user", Content: "Hello", Tier: TierHot},
			{Role: "assistant", Content: "Hi there, how can I help?", Tier: TierHot},
			{Role: "system", Content: "Old summary", Tier: TierWarm},
		},
	}
	count := session.HotTokenCount()
	if count == 0 {
		t.Error("expected non-zero hot token count")
	}
}

func TestPromoteAndApplySummary(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	session := &Session{
		Records: []Record{
			{Timestamp: now, Role: "user", Content: "First message", Tier: TierHot},
			{Timestamp: now.Add(time.Minute), Role: "assistant", Content: "First reply", Tier: TierHot},
			{Timestamp: now.Add(2 * time.Minute), Role: "user", Content: "Second message", Tier: TierHot},
			{Timestamp: now.Add(3 * time.Minute), Role: "assistant", Content: "Second reply", Tier: TierHot},
			{Timestamp: now.Add(4 * time.Minute), Role: "user", Content: "Third message", Tier: TierHot},
			{Timestamp: now.Add(5 * time.Minute), Role: "assistant", Content: "Third reply", Tier: TierHot},
		},
	}

	toSummarize := session.PromoteOldestHotToWarm(20)
	if len(toSummarize) == 0 {
		t.Fatal("expected records to summarize")
	}

	session.ApplySummary(toSummarize, "Summary of early conversation")

	// Check that warm record was created
	warmCount := 0
	hotCount := 0
	for _, r := range session.Records {
		if r.Tier == TierWarm {
			warmCount++
			if r.SummaryRange == nil {
				t.Error("warm record should have summary range")
			}
			if r.Content != "Summary of early conversation" {
				t.Errorf("unexpected summary: %s", r.Content)
			}
		}
		if r.Tier == TierHot {
			hotCount++
		}
	}
	if warmCount != 1 {
		t.Errorf("expected 1 warm record, got %d", warmCount)
	}
	if hotCount >= 6 {
		t.Errorf("expected fewer hot records after compaction, got %d", hotCount)
	}
}

func TestWarmRecord(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	record := Record{
		Timestamp: now,
		Role:      "summary",
		Content:   "Summary of conversation",
		Tier:      TierWarm,
		SummaryRange: &TimeRange{
			From: now.Add(-time.Hour),
			To:   now.Add(-time.Minute),
		},
	}

	if record.SummaryRange == nil {
		t.Fatal("expected summary range")
	}
	if record.Tier != TierWarm {
		t.Errorf("expected warm tier")
	}
}
