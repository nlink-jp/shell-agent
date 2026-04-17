package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Tier represents the memory tier of a record.
type Tier string

const (
	TierHot  Tier = "hot"
	TierWarm Tier = "warm"
	TierCold Tier = "cold"
)

// Record is a single memory entry with timestamp.
type Record struct {
	Timestamp    time.Time    `json:"timestamp"`
	Role         string       `json:"role"`
	Content      string       `json:"content"`
	Tier         Tier         `json:"tier"`
	SummaryRange *TimeRange   `json:"summary_range,omitempty"`
	Images       []ImageEntry `json:"images,omitempty"`
}

// TimeRange represents the time span of a summarized memory.
type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// ImageEntry holds a base64-encoded image for multimodal messages.
type ImageEntry struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

// Session represents a conversation session.
type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Records   []Record  `json:"records"`
}

// Store manages session persistence.
type Store struct {
	dir string
}

// NewStore creates a Store that persists sessions as JSON files.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Save persists a session to disk.
func (s *Store) Save(session *Session) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, session.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// Load reads a session from disk.
func (s *Store) Load(id string) (*Session, error) {
	path := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// List returns all session metadata (without full records).
func (s *Store) List() ([]Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		sess, err := s.Load(id)
		if err != nil {
			continue
		}
		sessions = append(sessions, Session{
			ID:        sess.ID,
			Title:     sess.Title,
			CreatedAt: sess.CreatedAt,
			UpdatedAt: sess.UpdatedAt,
		})
	}
	return sessions, nil
}
