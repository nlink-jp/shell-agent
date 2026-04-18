package memory

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var base64StdDecode = base64.StdEncoding.DecodeString
var base64StdEncode = base64.StdEncoding.EncodeToString

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
	InTokens     int          `json:"in_tokens,omitempty"`
	OutTokens    int          `json:"out_tokens,omitempty"`
}

// TimeRange represents the time span of a summarized memory.
type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// ImageEntry holds a reference to an image stored on disk.
type ImageEntry struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
}

// ImageStore manages image files on disk.
type ImageStore struct {
	dir string
}

// NewImageStore creates an ImageStore.
func NewImageStore(dir string) (*ImageStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &ImageStore{dir: dir}, nil
}

// Save writes image data to disk and returns the ID.
func (s *ImageStore) Save(dataURL string) (string, string, error) {
	// Parse data URL: data:image/png;base64,xxxx
	mimeType := "image/png"
	base64Data := dataURL

	if idx := indexOf(dataURL, ","); idx >= 0 {
		header := dataURL[:idx]
		base64Data = dataURL[idx+1:]
		// Extract mime type from "data:image/png;base64"
		if start := indexOf(header, ":"); start >= 0 {
			if end := indexOf(header, ";"); end > start {
				mimeType = header[start+1 : end]
			}
		}
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano())

	ext := ".png"
	switch mimeType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}

	data, err := base64Decode(base64Data)
	if err != nil {
		return "", "", fmt.Errorf("decode image: %w", err)
	}

	path := filepath.Join(s.dir, id+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", "", err
	}

	return id + ext, mimeType, nil
}

// LoadAsDataURL reads an image and returns it as a data URL.
func (s *ImageStore) LoadAsDataURL(id, mimeType string) (string, error) {
	path := filepath.Join(s.dir, id)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	encoded := base64Encode(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded), nil
}

// Path returns the filesystem path for an image.
func (s *ImageStore) Path(id string) string {
	return filepath.Join(s.dir, id)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func base64Decode(s string) ([]byte, error) {
	// Use encoding/base64 from stdlib
	return base64StdDecode(s)
}

func base64Encode(data []byte) string {
	return base64StdEncode(data)
}

// Session represents a conversation session.
type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Records   []Record  `json:"records"`
}

// EstimateTokens estimates the token count of a string.
// Uses max of char-based and word-based estimation for accuracy
// with mixed Japanese/English content.
func EstimateTokens(s string) int {
	charBased := len(s) / 2   // ~2 chars per token for CJK-heavy text
	wordBased := len(s) / 4   // ~4 chars per token for English
	if charBased > wordBased {
		return charBased
	}
	return wordBased
}

// HotTokenCount returns the total estimated tokens in hot records.
func (s *Session) HotTokenCount() int {
	total := 0
	for _, r := range s.Records {
		if r.Tier == TierHot {
			total += EstimateTokens(r.Content)
		}
	}
	return total
}

// HotRecords returns only hot tier records.
func (s *Session) HotRecords() []Record {
	var hot []Record
	for _, r := range s.Records {
		if r.Tier == TierHot {
			hot = append(hot, r)
		}
	}
	return hot
}

// PromoteOldestHotToWarm moves the oldest hot records to warm tier
// by replacing them with a summary record. Returns the records to summarize.
// The caller is responsible for generating the summary via LLM and calling
// ApplySummary with the result.
func (s *Session) PromoteOldestHotToWarm(targetTokenReduction int) []Record {
	var toSummarize []Record
	tokens := 0
	for i, r := range s.Records {
		if r.Tier != TierHot {
			continue
		}
		t := EstimateTokens(r.Content)
		toSummarize = append(toSummarize, r)
		tokens += t
		// Keep at least 2 hot messages (latest user + assistant)
		remaining := 0
		for _, rr := range s.Records[i+1:] {
			if rr.Tier == TierHot {
				remaining++
			}
		}
		if remaining <= 2 || tokens >= targetTokenReduction {
			break
		}
	}
	return toSummarize
}

// ApplySummary replaces the given hot records with a single warm summary.
func (s *Session) ApplySummary(summarized []Record, summaryText string) {
	if len(summarized) == 0 {
		return
	}

	timeFrom := summarized[0].Timestamp
	timeTo := summarized[len(summarized)-1].Timestamp

	// Build set of records to remove
	removeSet := make(map[int]bool)
	for _, sr := range summarized {
		for i, r := range s.Records {
			if r.Timestamp.Equal(sr.Timestamp) && r.Role == sr.Role && r.Content == sr.Content {
				removeSet[i] = true
				break
			}
		}
	}

	// Rebuild records: warm/cold first, then new summary, then remaining hot
	var newRecords []Record
	for i, r := range s.Records {
		if removeSet[i] {
			continue
		}
		if r.Tier == TierWarm || r.Tier == TierCold {
			newRecords = append(newRecords, r)
		}
	}

	// Insert the new warm summary
	newRecords = append(newRecords, Record{
		Timestamp: timeTo,
		Role:      "system",
		Content:   summaryText,
		Tier:      TierWarm,
		SummaryRange: &TimeRange{
			From: timeFrom,
			To:   timeTo,
		},
	})

	// Add remaining hot records
	for i, r := range s.Records {
		if removeSet[i] {
			continue
		}
		if r.Tier == TierHot {
			newRecords = append(newRecords, r)
		}
	}

	s.Records = newRecords
}

// PinnedMemory represents an important fact extracted by the LLM.
type PinnedMemory struct {
	Fact       string    `json:"fact"`        // English (for analysis)
	NativeFact string    `json:"native_fact"` // User's native language
	Category   string    `json:"category"`    // preference, decision, fact, context
	SourceTime time.Time `json:"source_time"`
	CreatedAt  time.Time `json:"created_at"`
}

// PinnedStore manages cross-session persistent memories.
type PinnedStore struct {
	path    string
	Entries []PinnedMemory `json:"entries"`
}

// NewPinnedStore creates or loads a PinnedStore.
func NewPinnedStore(path string) (*PinnedStore, error) {
	ps := &PinnedStore{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ps, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &ps.Entries); err != nil {
		return nil, err
	}
	return ps, nil
}

// Save persists pinned memories to disk.
func (ps *PinnedStore) Save() error {
	data, err := json.MarshalIndent(ps.Entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ps.path, data, 0o644)
}

// Add appends a new pinned memory, deduplicating by fact content.
func (ps *PinnedStore) Add(m PinnedMemory) bool {
	for _, e := range ps.Entries {
		if e.Fact == m.Fact {
			return false
		}
	}
	ps.Entries = append(ps.Entries, m)
	return true
}

// Update replaces a pinned memory at the given index.
func (ps *PinnedStore) Update(index int, m PinnedMemory) bool {
	if index < 0 || index >= len(ps.Entries) {
		return false
	}
	ps.Entries[index] = m
	return true
}

// Delete removes a pinned memory at the given index.
func (ps *PinnedStore) Delete(index int) bool {
	if index < 0 || index >= len(ps.Entries) {
		return false
	}
	ps.Entries = append(ps.Entries[:index], ps.Entries[index+1:]...)
	return true
}

// FormatForPrompt returns pinned memories as a string for system prompt injection.
func (ps *PinnedStore) FormatForPrompt() string {
	if len(ps.Entries) == 0 {
		return ""
	}
	var s string
	for _, e := range ps.Entries {
		s += "- [" + e.Category + "] " + e.Fact
		if e.NativeFact != "" && e.NativeFact != e.Fact {
			s += " (" + e.NativeFact + ")"
		}
		s += "\n"
	}
	return s
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

// Delete removes a session from disk.
func (s *Store) Delete(id string) error {
	path := filepath.Join(s.dir, id+".json")
	return os.Remove(path)
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
