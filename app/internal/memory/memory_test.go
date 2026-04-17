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
