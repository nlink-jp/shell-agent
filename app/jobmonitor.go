package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nlink-jp/shell-agent/internal/analysis"
	"github.com/nlink-jp/shell-agent/internal/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// JobMonitor tracks background async jobs and emits events on state changes.
// Currently used for analysis jobs; designed to support any async task type.
type JobMonitor struct {
	mu       sync.Mutex
	jobs     map[string]*monitoredJob
	ctx      context.Context
	resultCh chan jobCompletionResponse
}

type monitoredJob struct {
	JobID      string    `json:"job_id"`
	Prompt     string    `json:"prompt"`
	StartedAt  time.Time `json:"started_at"`
	Progress   string    `json:"progress"`
	State      string    `json:"state"` // "running", "done", "error"
	Reviewed   bool      `json:"reviewed"`
}

type jobCompletionResponse struct {
	jobID    string
	accepted bool
}

// JobCardInfo is exposed to the frontend.
type JobCardInfo struct {
	JobID     string `json:"job_id"`
	Prompt    string `json:"prompt"`
	StartedAt string `json:"started_at"`
	Progress  string `json:"progress"`
	State     string `json:"state"`
	Reviewed  bool   `json:"reviewed"`
}

func newJobMonitor() *JobMonitor {
	return &JobMonitor{
		jobs:     make(map[string]*monitoredJob),
		resultCh: make(chan jobCompletionResponse, 1),
	}
}

// SetContext sets the Wails context for event emission.
func (m *JobMonitor) SetContext(ctx context.Context) {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
}

// Track starts monitoring a background job.
func (m *JobMonitor) Track(jobID, prompt, outputDir string) {
	m.mu.Lock()
	m.jobs[jobID] = &monitoredJob{
		JobID:     jobID,
		Prompt:    prompt,
		StartedAt: time.Now(),
		State:     "running",
		Progress:  "starting",
	}
	m.mu.Unlock()

	// Notify frontend
	wailsRuntime.EventsEmit(m.ctx, "chat:job_started", map[string]string{
		"job_id": jobID,
		"prompt": prompt,
	})

	go m.poll(jobID, outputDir)
}

func (m *JobMonitor) poll(jobID, outputDir string) {
	log := logger.New("jobmon")
	statusPath := filepath.Join(outputDir, "status.json")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastProgress := ""

	for {
		select {
		case <-ticker.C:
			data, err := os.ReadFile(statusPath)
			if err != nil {
				continue
			}
			var status analysis.JobStatus
			if err := json.Unmarshal(data, &status); err != nil {
				continue
			}

			m.mu.Lock()
			job, ok := m.jobs[jobID]
			if !ok {
				m.mu.Unlock()
				return
			}
			job.Progress = status.Progress
			job.State = status.State
			m.mu.Unlock()

			// Emit progress if changed
			if status.Progress != lastProgress {
				lastProgress = status.Progress
				wailsRuntime.EventsEmit(m.ctx, "chat:job_progress", map[string]string{
					"job_id":   jobID,
					"progress": status.Progress,
					"state":    status.State,
				})
			}

			// Terminal state
			if status.State == "done" || status.State == "error" {
				log.Info("job %s: %s (%s)", jobID, status.State, status.Progress)

				if status.State == "done" {
					// Read findings count for the notification
					findingCount := 0
					findingsPath := filepath.Join(outputDir, "findings.json")
					if fdata, err := os.ReadFile(findingsPath); err == nil {
						var findings []analysis.Finding
						if json.Unmarshal(fdata, &findings) == nil {
							findingCount = len(findings)
						}
					}

					wailsRuntime.EventsEmit(m.ctx, "chat:job_completed_ask", map[string]any{
						"job_id":        jobID,
						"prompt":        job.Prompt,
						"finding_count": findingCount,
						"progress":      status.Progress,
					})
				} else {
					wailsRuntime.EventsEmit(m.ctx, "chat:job_error", map[string]string{
						"job_id": jobID,
						"error":  status.Error,
					})
				}
				return
			}
		}
	}
}

// AcceptJobResult is called from the frontend when user clicks "Review now".
func (m *JobMonitor) AcceptJobResult(jobID string) {
	m.mu.Lock()
	if job, ok := m.jobs[jobID]; ok {
		job.Reviewed = true
	}
	m.mu.Unlock()

	select {
	case m.resultCh <- jobCompletionResponse{jobID: jobID, accepted: true}:
	default:
	}
}

// DeferJobResult is called from the frontend when user clicks "Later".
func (m *JobMonitor) DeferJobResult(jobID string) {
	select {
	case m.resultCh <- jobCompletionResponse{jobID: jobID, accepted: false}:
	default:
	}
}

// GetJobs returns all tracked jobs for the frontend.
func (m *JobMonitor) GetJobs() []JobCardInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []JobCardInfo
	for _, j := range m.jobs {
		result = append(result, JobCardInfo{
			JobID:     j.JobID,
			Prompt:    j.Prompt,
			StartedAt: j.StartedAt.Format("15:04:05"),
			Progress:  j.Progress,
			State:     j.State,
			Reviewed:  j.Reviewed,
		})
	}
	return result
}

// RemoveJob removes a completed job from the monitor.
func (m *JobMonitor) RemoveJob(jobID string) {
	m.mu.Lock()
	delete(m.jobs, jobID)
	m.mu.Unlock()
}
