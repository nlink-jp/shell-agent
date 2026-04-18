package analysis

import (
	"context"
	"fmt"

	"github.com/nlink-jp/shell-agent/internal/client"
)

// ClientAdapter adapts client.Client to the LLMClient interface.
type ClientAdapter struct {
	c *client.Client
}

// NewClientAdapter wraps an existing OpenAI-compatible client.
func NewClientAdapter(c *client.Client) *ClientAdapter {
	return &ClientAdapter{c: c}
}

// Chat sends a system+user prompt pair and returns the assistant's text response.
func (a *ClientAdapter) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []client.Message{
		client.TextMessage("system", systemPrompt),
		client.TextMessage("user", userPrompt),
	}

	resp, err := a.c.ChatWithContext(ctx, messages, nil)
	if err != nil {
		return "", fmt.Errorf("llm chat: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}

	return resp.Choices[0].Message.Content, nil
}
