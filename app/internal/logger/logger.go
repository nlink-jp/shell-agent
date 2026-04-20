// Package logger provides structured logging for shell-agent.
//
// Logs are written to both stderr (for terminal diagnostics) and a rotating
// log file at ~/Library/Application Support/shell-agent/logs/app.log.
// Each line is prefixed with a timestamp and component tag.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Logger writes timestamped, component-tagged log lines.
type Logger struct {
	component string
	logger    *log.Logger
}

var (
	initOnce sync.Once
	writer   io.Writer = os.Stderr
	logFile  *os.File
)

// Init sets up the log output to both stderr and the log file.
// Safe to call multiple times; only the first call takes effect.
func Init(logDir string) {
	initOnce.Do(func() {
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return
		}
		logPath := filepath.Join(logDir, "app.log")

		// Rotate if over 10MB
		if info, err := os.Stat(logPath); err == nil && info.Size() > 10*1024*1024 {
			prev := logPath + ".prev"
			_ = os.Remove(prev)
			_ = os.Rename(logPath, prev)
		}

		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		logFile = f
		writer = io.MultiWriter(os.Stderr, f)
	})
}

// Close flushes and closes the log file. Call on shutdown.
func Close() {
	if logFile != nil {
		logFile.Close()
	}
}

// New creates a logger for a named component.
func New(component string) *Logger {
	prefix := fmt.Sprintf("[%s] ", component)
	return &Logger{
		component: component,
		logger:    log.New(writer, prefix, log.LstdFlags),
	}
}

// Info logs an informational message.
func (l *Logger) Info(format string, args ...any) {
	l.logger.Printf("INFO  "+format, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(format string, args ...any) {
	l.logger.Printf("WARN  "+format, args...)
}

// Error logs an error message.
func (l *Logger) Error(format string, args ...any) {
	l.logger.Printf("ERROR "+format, args...)
}

// Debug logs a debug message.
func (l *Logger) Debug(format string, args ...any) {
	l.logger.Printf("DEBUG "+format, args...)
}
