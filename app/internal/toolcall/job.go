package toolcall

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Job represents a single tool execution with its own workspace.
type Job struct {
	ID      string
	WorkDir string
}

// Artifact is a file produced by a tool execution.
type Artifact struct {
	Name     string
	MimeType string
	Data     []byte
}

// JobManager creates and manages tool execution jobs.
type JobManager struct{}

// NewJobManager creates a JobManager.
func NewJobManager() *JobManager {
	return &JobManager{}
}

// NewJob creates a new job with a unique ID and temporary workspace.
func (jm *JobManager) NewJob() (*Job, error) {
	id := fmt.Sprintf("job-%d", time.Now().UnixNano())
	workDir, err := os.MkdirTemp("", "shell-agent-"+id+"-")
	if err != nil {
		return nil, err
	}
	return &Job{
		ID:      id,
		WorkDir: workDir,
	}, nil
}

// Finalize reads any files produced in the work directory and returns them as artifacts.
// The caller is responsible for saving artifacts to the object store.
// The temp directory is cleaned up after this call.
func (jm *JobManager) Finalize(job *Job) ([]Artifact, error) {
	entries, err := os.ReadDir(job.WorkDir)
	if err != nil {
		_ = os.RemoveAll(job.WorkDir)
		return nil, err
	}

	var artifacts []Artifact
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(job.WorkDir, e.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}

		mimeType := detectMime(e.Name())
		artifacts = append(artifacts, Artifact{
			Name:     e.Name(),
			MimeType: mimeType,
			Data:     data,
		})
	}

	// Clean up temp directory
	_ = os.RemoveAll(job.WorkDir)

	return artifacts, nil
}

// detectMime guesses MIME type from file extension.
func detectMime(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if mt := mime.TypeByExtension(ext); mt != "" {
		return mt
	}
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".md":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
