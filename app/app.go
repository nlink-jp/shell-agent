package main

import (
	"context"
	"fmt"
	"time"

	"github.com/nlink-jp/shell-agent/internal/client"
	"github.com/nlink-jp/shell-agent/internal/config"
	"github.com/nlink-jp/shell-agent/internal/memory"
	"github.com/nlink-jp/shell-agent/internal/toolcall"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ChatMessage is exposed to the frontend.
type ChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// ToolInfo is exposed to the frontend.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// SessionInfo is exposed to the frontend for the sidebar.
type SessionInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
}

// LLMStatus is exposed to the frontend for the monitoring panel.
type LLMStatus struct {
	CurrentTime  string `json:"current_time"`
	HotMessages  int    `json:"hot_messages"`
	WarmSummaries int   `json:"warm_summaries"`
	ColdSummaries int   `json:"cold_summaries"`
	TokensUsed   int    `json:"tokens_used"`
	TokenLimit   int    `json:"token_limit"`
}

// App is the main application struct exposed to the frontend via Wails bindings.
type App struct {
	ctx      context.Context
	cfg      *config.Config
	llm      *client.Client
	store    *memory.Store
	tools    *toolcall.Registry
	session  *memory.Session
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("config load error: %v\n", err)
		cfg = config.DefaultConfig()
	}
	a.cfg = cfg

	a.llm = client.New(cfg.API.Endpoint, cfg.API.Model, cfg.API.APIKey)

	store, err := memory.NewStore(config.ConfigDir() + "/sessions")
	if err != nil {
		fmt.Printf("store init error: %v\n", err)
	}
	a.store = store

	a.tools = toolcall.NewRegistry(cfg.Tools.ScriptDir)
	_ = a.tools.Scan()

	a.NewSession()
}

// shutdown is called when the app is closing.
func (a *App) shutdown(_ context.Context) {
	if a.session != nil && a.store != nil {
		_ = a.store.Save(a.session)
	}
}

// NewSession creates a new chat session.
func (a *App) NewSession() string {
	now := time.Now()
	a.session = &memory.Session{
		ID:        fmt.Sprintf("session-%d", now.UnixMilli()),
		Title:     "New Chat",
		CreatedAt: now,
		UpdatedAt: now,
	}
	return a.session.ID
}

// LoadSession loads an existing session.
func (a *App) LoadSession(id string) ([]ChatMessage, error) {
	sess, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	a.session = sess

	var msgs []ChatMessage
	for _, r := range sess.Records {
		msgs = append(msgs, ChatMessage{
			Role:      r.Role,
			Content:   r.Content,
			Timestamp: r.Timestamp.Format("15:04:05"),
		})
	}
	return msgs, nil
}

// ListSessions returns all sessions for the sidebar.
func (a *App) ListSessions() ([]SessionInfo, error) {
	sessions, err := a.store.List()
	if err != nil {
		return nil, err
	}
	var infos []SessionInfo
	for _, s := range sessions {
		infos = append(infos, SessionInfo{
			ID:        s.ID,
			Title:     s.Title,
			UpdatedAt: s.UpdatedAt.Format("2006-01-02 15:04"),
		})
	}
	return infos, nil
}

// SendMessage sends a user message and streams the LLM response.
func (a *App) SendMessage(content string) (ChatMessage, error) {
	now := time.Now()

	// Add user message to hot memory
	a.session.Records = append(a.session.Records, memory.Record{
		Timestamp: now,
		Role:      "user",
		Content:   content,
		Tier:      memory.TierHot,
	})

	// Build messages with time context
	systemPrompt := "You are a helpful assistant. You have access to tools that can execute shell scripts. Respond concisely."
	messages := a.buildMessages(systemPrompt)

	// Build tool definitions
	tools := a.buildToolDefs()

	// Stream response
	var fullResponse string
	err := a.llm.ChatStream(messages, tools, func(token string, toolCalls []client.ToolCall, done bool) {
		if token != "" {
			fullResponse += token
			wailsRuntime.EventsEmit(a.ctx, "chat:token", token)
		}
		if len(toolCalls) > 0 {
			wailsRuntime.EventsEmit(a.ctx, "chat:toolcall", toolCalls)
		}
		if done {
			wailsRuntime.EventsEmit(a.ctx, "chat:done", nil)
		}
	})
	if err != nil {
		return ChatMessage{}, err
	}

	// Add assistant response to hot memory
	respTime := time.Now()
	a.session.Records = append(a.session.Records, memory.Record{
		Timestamp: respTime,
		Role:      "assistant",
		Content:   fullResponse,
		Tier:      memory.TierHot,
	})
	a.session.UpdatedAt = respTime

	// Auto-save
	if a.store != nil {
		_ = a.store.Save(a.session)
	}

	return ChatMessage{
		Role:      "assistant",
		Content:   fullResponse,
		Timestamp: respTime.Format("15:04:05"),
	}, nil
}

// GetTools returns all registered tools.
func (a *App) GetTools() []ToolInfo {
	var infos []ToolInfo
	for _, t := range a.tools.List() {
		infos = append(infos, ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			Category:    string(t.Category),
		})
	}
	return infos
}

// GetLLMStatus returns the current LLM state for monitoring.
func (a *App) GetLLMStatus() LLMStatus {
	var hot, warm, cold int
	if a.session != nil {
		for _, r := range a.session.Records {
			switch r.Tier {
			case memory.TierHot:
				hot++
			case memory.TierWarm:
				warm++
			case memory.TierCold:
				cold++
			}
		}
	}
	return LLMStatus{
		CurrentTime:   time.Now().Format("2006-01-02 15:04:05"),
		HotMessages:   hot,
		WarmSummaries: warm,
		ColdSummaries: cold,
		TokenLimit:    a.cfg.Memory.HotTokenLimit,
	}
}

// ApproveTool is called when the user approves a MITL tool call.
func (a *App) ApproveTool(toolName, argsJSON string) (string, error) {
	return a.tools.Execute(toolName, argsJSON)
}

// RejectTool is called when the user rejects a MITL tool call.
func (a *App) RejectTool(toolName string) string {
	return fmt.Sprintf("Tool call '%s' was rejected by the user.", toolName)
}

func (a *App) buildMessages(systemPrompt string) []client.Message {
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

	for _, r := range a.session.Records {
		switch r.Tier {
		case memory.TierWarm, memory.TierCold:
			if r.SummaryRange != nil {
				summary := fmt.Sprintf("[Memory %s — %s]\n%s",
					r.SummaryRange.From.Format("15:04:05"),
					r.SummaryRange.To.Format("15:04:05"),
					r.Content,
				)
				messages = append(messages, client.TextMessage("system", summary))
			}
		case memory.TierHot:
			content := fmt.Sprintf("[%s] %s", r.Timestamp.Format("15:04:05"), r.Content)
			messages = append(messages, client.TextMessage(r.Role, content))
		}
	}
	return messages
}

func (a *App) buildToolDefs() []client.Tool {
	var tools []client.Tool
	for _, t := range a.tools.List() {
		props := make(map[string]any)
		var required []string
		for _, p := range t.Params {
			props[p.Name] = map[string]any{
				"type":        p.Type,
				"description": p.Description,
			}
			required = append(required, p.Name)
		}
		tools = append(tools, client.Tool{
			Type: "function",
			Function: client.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters: map[string]any{
					"type":       "object",
					"properties": props,
					"required":   required,
				},
			},
		})
	}
	return tools
}
