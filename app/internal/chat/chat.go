package chat

import (
	"fmt"
	"time"

	"github.com/nlink-jp/shell-agent/internal/client"
	"github.com/nlink-jp/shell-agent/internal/memory"
)

// Engine orchestrates chat sessions with memory management.
type Engine struct {
	client        *client.Client
	hotTokenLimit int
}

// NewEngine creates a chat Engine.
func NewEngine(c *client.Client, hotTokenLimit int) *Engine {
	return &Engine{
		client:        c,
		hotTokenLimit: hotTokenLimit,
	}
}

// BuildMessages constructs the message array for the API call,
// injecting current time and memory context.
func (e *Engine) BuildMessages(session *memory.Session, systemPrompt string) []client.Message {
	now := time.Now()

	timeContext := fmt.Sprintf(
		"Current date and time: %s\nTimezone: %s",
		now.Format("2006-01-02 15:04:05"),
		now.Location().String(),
	)

	fullSystem := fmt.Sprintf("%s\n\n%s", systemPrompt, timeContext)
	messages := []client.Message{
		client.TextMessage("system", fullSystem),
	}

	// Add warm/cold summaries first
	for _, r := range session.Records {
		if r.Tier == memory.TierWarm || r.Tier == memory.TierCold {
			summary := fmt.Sprintf("[Memory from %s to %s]\n%s",
				r.SummaryRange.From.Format("2006-01-02 15:04:05"),
				r.SummaryRange.To.Format("2006-01-02 15:04:05"),
				r.Content,
			)
			messages = append(messages, client.TextMessage("system", summary))
		}
	}

	// Add hot messages with timestamps
	for _, r := range session.Records {
		if r.Tier != memory.TierHot {
			continue
		}
		content := fmt.Sprintf("[%s] %s", r.Timestamp.Format("15:04:05"), r.Content)
		messages = append(messages, client.TextMessage(r.Role, content))
	}

	return messages
}
