// Package objstore provides a central object repository for all binary data
// (images, blobs, reports, etc.) with unique ID-based storage and metadata tracking.
package objstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ObjectType represents the type of stored object.
type ObjectType string

const (
	TypeImage  ObjectType = "image"
	TypeBlob   ObjectType = "blob"
	TypeReport ObjectType = "report"
)

// ObjectMeta holds metadata about a stored object.
type ObjectMeta struct {
	ID        string     `json:"id"`
	Type      ObjectType `json:"type"`
	MimeType  string     `json:"mime_type"`
	Filename  string     `json:"filename"`
	CreatedAt time.Time  `json:"created_at"`
	SessionID string     `json:"session_id,omitempty"`
	Size      int64      `json:"size"`
}

// Store is the central object repository.
type Store struct {
	dir     string
	dataDir string
	index   map[string]*ObjectMeta
	mu      sync.RWMutex
}

// New creates or opens an object store at the given directory.
func New(dir string) (*Store, error) {
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	s := &Store{
		dir:     dir,
		dataDir: dataDir,
		index:   make(map[string]*ObjectMeta),
	}

	// Load existing index
	if err := s.loadIndex(); err != nil {
		// Index doesn't exist or is corrupted — rebuild from files
		s.rebuildIndex()
	}

	return s, nil
}

// generateID creates a unique object ID.
func generateID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// extFromMime returns a file extension for a MIME type.
func extFromMime(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/png":
		return ".png"
	case "text/markdown":
		return ".md"
	case "text/plain":
		return ".txt"
	default:
		if strings.HasPrefix(mime, "image/") {
			return ".png"
		}
		return ".bin"
	}
}

// Save stores binary data and returns the object ID.
func (s *Store) Save(data []byte, objType ObjectType, mimeType, filename string) (string, error) {
	id := generateID()
	ext := extFromMime(mimeType)
	storedName := id + ext

	path := filepath.Join(s.dataDir, storedName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write object: %w", err)
	}

	meta := &ObjectMeta{
		ID:        id,
		Type:      objType,
		MimeType:  mimeType,
		Filename:  filename,
		CreatedAt: time.Now(),
		Size:      int64(len(data)),
	}

	s.mu.Lock()
	s.index[id] = meta
	s.mu.Unlock()

	_ = s.saveIndex()
	return id, nil
}

// SaveDataURL parses a data URL, stores the binary data, and returns the object ID.
func (s *Store) SaveDataURL(dataURL string, objType ObjectType, filename string) (string, error) {
	mimeType, data, err := parseDataURL(dataURL)
	if err != nil {
		return "", err
	}
	return s.Save(data, objType, mimeType, filename)
}

// Load reads the binary data of an object.
func (s *Store) Load(id string) ([]byte, error) {
	s.mu.RLock()
	meta, ok := s.index[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("object not found: %s", id)
	}

	path := filepath.Join(s.dataDir, id+extFromMime(meta.MimeType))
	return os.ReadFile(path)
}

// LoadAsDataURL returns the object as a base64 data URL.
func (s *Store) LoadAsDataURL(id string) (string, error) {
	s.mu.RLock()
	meta, ok := s.index[id]
	s.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("object not found: %s", id)
	}

	data, err := s.Load(id)
	if err != nil {
		return "", err
	}

	encoded := encodeBase64(data)
	return fmt.Sprintf("data:%s;base64,%s", meta.MimeType, encoded), nil
}

// Get returns the metadata for an object.
func (s *Store) Get(id string) (*ObjectMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	meta, ok := s.index[id]
	return meta, ok
}

// Path returns the filesystem path for an object.
func (s *Store) Path(id string) string {
	s.mu.RLock()
	meta, ok := s.index[id]
	s.mu.RUnlock()
	if !ok {
		return ""
	}
	return filepath.Join(s.dataDir, id+extFromMime(meta.MimeType))
}

// List returns all objects of a given type.
func (s *Store) List(objType ObjectType) []*ObjectMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*ObjectMeta
	for _, m := range s.index {
		if m.Type == objType {
			result = append(result, m)
		}
	}
	return result
}

// Delete removes an object.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	meta, ok := s.index[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("object not found: %s", id)
	}
	delete(s.index, id)
	s.mu.Unlock()

	path := filepath.Join(s.dataDir, id+extFromMime(meta.MimeType))
	_ = os.Remove(path)
	return s.saveIndex()
}

// indexPath returns the path to the index file.
func (s *Store) indexPath() string {
	return filepath.Join(s.dir, "index.json")
}

// loadIndex reads the index from disk.
func (s *Store) loadIndex() error {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		return err
	}

	var entries []*ObjectMeta
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		s.index[e.ID] = e
	}
	return nil
}

// saveIndex writes the index to disk.
func (s *Store) saveIndex() error {
	s.mu.RLock()
	entries := make([]*ObjectMeta, 0, len(s.index))
	for _, m := range s.index {
		entries = append(entries, m)
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), data, 0o644)
}

// rebuildIndex scans the data directory and rebuilds the index.
// Must NOT be called while holding any lock.
func (s *Store) rebuildIndex() {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return
	}

	s.mu.Lock()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		id := strings.TrimSuffix(name, ext)

		info, _ := e.Info()
		var size int64
		if info != nil {
			size = info.Size()
		}

		mime := "application/octet-stream"
		switch ext {
		case ".png":
			mime = "image/png"
		case ".jpg", ".jpeg":
			mime = "image/jpeg"
		case ".gif":
			mime = "image/gif"
		case ".webp":
			mime = "image/webp"
		case ".md":
			mime = "text/markdown"
		}

		s.index[id] = &ObjectMeta{
			ID:       id,
			Type:     TypeImage, // assume image for rebuilt entries
			MimeType: mime,
			Filename: name,
			Size:     size,
		}
	}

	s.mu.Unlock()
	_ = s.saveIndex()
}

// parseDataURL extracts mime type and binary data from a data URL.
func parseDataURL(dataURL string) (string, []byte, error) {
	// Format: data:image/png;base64,xxxx
	commaIdx := strings.Index(dataURL, ",")
	if commaIdx < 0 {
		return "", nil, fmt.Errorf("invalid data URL: no comma")
	}

	header := dataURL[:commaIdx]
	encoded := dataURL[commaIdx+1:]

	mimeType := "application/octet-stream"
	if colonIdx := strings.Index(header, ":"); colonIdx >= 0 {
		if semiIdx := strings.Index(header, ";"); semiIdx > colonIdx {
			mimeType = header[colonIdx+1 : semiIdx]
		}
	}

	data, err := decodeBase64(encoded)
	if err != nil {
		return "", nil, fmt.Errorf("decode base64: %w", err)
	}

	return mimeType, data, nil
}
