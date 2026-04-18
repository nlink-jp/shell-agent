package toolcall

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Job represents a single tool execution with its own workspace.
type Job struct {
	ID      string
	WorkDir string
	BlobDir string
}

// JobManager creates and manages tool execution jobs.
type JobManager struct {
	blobRoot string // persistent storage for job artifacts
}

// NewJobManager creates a JobManager.
func NewJobManager(blobRoot string) (*JobManager, error) {
	if err := os.MkdirAll(blobRoot, 0o755); err != nil {
		return nil, err
	}
	return &JobManager{blobRoot: blobRoot}, nil
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
		BlobDir: filepath.Join(jm.blobRoot, id),
	}, nil
}

// Finalize moves any files produced in the work directory to persistent blob storage.
// Returns a list of blob paths (relative to blobRoot).
func (jm *JobManager) Finalize(job *Job) ([]string, error) {
	entries, err := os.ReadDir(job.WorkDir)
	if err != nil {
		return nil, err
	}

	var blobs []string
	if len(entries) > 0 {
		if err := os.MkdirAll(job.BlobDir, 0o755); err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			src := filepath.Join(job.WorkDir, e.Name())
			dst := filepath.Join(job.BlobDir, e.Name())
			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				continue
			}
			blobs = append(blobs, filepath.Join(job.ID, e.Name()))
		}
	}

	// Clean up temp directory
	_ = os.RemoveAll(job.WorkDir)

	return blobs, nil
}

// BlobPath returns the full filesystem path for a blob reference.
func (jm *JobManager) BlobPath(blobRef string) string {
	return filepath.Join(jm.blobRoot, blobRef)
}
