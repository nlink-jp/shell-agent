package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client communicates with an OpenAI-compatible API.
type Client struct {
	endpoint   string
	model      string
	apiKey     string
	httpClient *http.Client
}

// New creates a new Client.
func New(endpoint, model, apiKey string) *Client {
	return &Client{
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		apiKey:   apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// Message represents a chat message.
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// ContentPart represents a part of a multimodal message.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL holds a base64 data URL for an image.
type ImageURL struct {
	URL string `json:"url"`
}

// TextMessage creates a simple text message.
func TextMessage(role, content string) Message {
	return Message{Role: role, Content: content}
}

// ImageMessage creates a message with text and images.
// Each image is preceded by a label to help VLMs distinguish multiple images.
func ImageMessage(role, text string, images []string, labels []string) Message {
	var parts []ContentPart
	for i, dataURL := range images {
		label := fmt.Sprintf("[Image %d]", i+1)
		if i < len(labels) && labels[i] != "" {
			label = labels[i]
		}
		parts = append(parts,
			ContentPart{Type: "text", Text: label},
			ContentPart{Type: "image_url", ImageURL: &ImageURL{URL: dataURL}},
		)
	}
	if text != "" {
		parts = append(parts, ContentPart{Type: "text", Text: text})
	}
	return Message{Role: role, Content: parts}
}

// ChatRequest is the request payload for /chat/completions.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
}

// Tool represents a function tool definition.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function.
type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ChatResponse is the response from /chat/completions (non-streaming).
type ChatResponse struct {
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Usage holds token consumption information.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Choice represents one completion choice.
type Choice struct {
	Message      ResponseMessage `json:"message"`
	Delta        ResponseMessage `json:"delta"`
	FinishReason string          `json:"finish_reason"`
}

// ResponseMessage is the assistant's response message.
type ResponseMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a function call request from the LLM.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the function name and arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// StreamCallback is called for each streamed token.
type StreamCallback func(token string, toolCalls []ToolCall, done bool)

// Chat sends a non-streaming chat request.
func (c *Client) Chat(messages []Message, tools []Tool) (*ChatResponse, error) {
	return c.ChatWithContext(context.Background(), messages, tools)
}

// ChatWithContext sends a non-streaming chat request with cancellation support.
func (c *Client) ChatWithContext(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	req := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Tools:    tools,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.doRequestCtx(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &chatResp, nil
}

// ChatStream sends a streaming chat request, calling cb for each token.
func (c *Client) ChatStream(messages []Message, tools []Tool, cb StreamCallback) error {
	req := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
		Tools:    tools,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.doRequest(body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			cb("", nil, true)
			return nil
		}

		var chunk struct {
			Choices []Choice `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			cb(delta.Content, delta.ToolCalls, chunk.Choices[0].FinishReason == "stop")
		}
	}
	return scanner.Err()
}

func (c *Client) doRequest(body []byte) (*http.Response, error) {
	return c.doRequestCtx(context.Background(), body)
}

func (c *Client) doRequestCtx(ctx context.Context, body []byte) (*http.Response, error) {
	url := c.endpoint + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return resp, nil
}
