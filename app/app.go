package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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
	CurrentTime   string `json:"current_time"`
	HotMessages   int    `json:"hot_messages"`
	WarmSummaries int    `json:"warm_summaries"`
	ColdSummaries int    `json:"cold_summaries"`
	TokensUsed    int    `json:"tokens_used"`
	TokenLimit    int    `json:"token_limit"`
}

// ToolCallRequest is sent to the frontend for MITL approval.
type ToolCallRequest struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Category  string `json:"category"`
	NeedsMITL bool   `json:"needs_mitl"`
}

// App is the main application struct exposed to the frontend via Wails bindings.
type App struct {
	ctx     context.Context
	cfg     *config.Config
	llm     *client.Client
	store   *memory.Store
	pinned  *memory.PinnedStore
	tools   *toolcall.Registry
	session *memory.Session

	// MITL approval channel
	mitlCh   chan mitlResponse
	mitlOnce sync.Once
}

type mitlResponse struct {
	approved bool
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{
		mitlCh: make(chan mitlResponse, 1),
	}
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

	if cfg.Window.Width > 0 && cfg.Window.Height > 0 {
		wailsRuntime.WindowSetSize(ctx, cfg.Window.Width, cfg.Window.Height)
		wailsRuntime.WindowSetPosition(ctx, cfg.Window.X, cfg.Window.Y)
	}

	a.llm = client.New(cfg.API.Endpoint, cfg.API.Model, cfg.API.APIKey)

	store, err := memory.NewStore(config.ConfigDir() + "/sessions")
	if err != nil {
		fmt.Printf("store init error: %v\n", err)
	}
	a.store = store

	pinned, err := memory.NewPinnedStore(config.ConfigDir() + "/pinned.json")
	if err != nil {
		fmt.Printf("pinned store error: %v\n", err)
		pinned = &memory.PinnedStore{}
	}
	a.pinned = pinned

	a.tools = toolcall.NewRegistry(cfg.Tools.ScriptDir)
	_ = a.tools.Scan()

	a.NewSession()
}

// shutdown is called when the app is closing.
func (a *App) shutdown(_ context.Context) {
	if a.session != nil && a.store != nil {
		_ = a.store.Save(a.session)
	}

	if a.ctx != nil && a.cfg != nil {
		w, h := wailsRuntime.WindowGetSize(a.ctx)
		x, y := wailsRuntime.WindowGetPosition(a.ctx)
		a.cfg.Window = config.WindowConfig{
			X: x, Y: y, Width: w, Height: h,
		}
		_ = a.cfg.Save()
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

// DeleteSession deletes a session by ID.
func (a *App) DeleteSession(id string) error {
	if a.session != nil && a.session.ID == id {
		a.NewSession()
	}
	return a.store.Delete(id)
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

// SendMessage sends a user message and runs the agent loop.
func (a *App) SendMessage(content string) (ChatMessage, error) {
	now := time.Now()

	a.session.Records = append(a.session.Records, memory.Record{
		Timestamp: now,
		Role:      "user",
		Content:   content,
		Tier:      memory.TierHot,
	})

	systemPrompt := "You are a helpful assistant. You have access to tools that can execute shell scripts. Respond concisely."
	toolDefs := a.buildToolDefs()

	const maxIterations = 10
	toolsExecuted := false
	for i := 0; i < maxIterations; i++ {
		messages := a.buildMessages(systemPrompt)

		// After tool execution, omit tool defs to force a text response
		var currentTools []client.Tool
		if !toolsExecuted {
			currentTools = toolDefs
		}

		resp, err := a.llm.Chat(messages, currentTools)
		if err != nil {
			return ChatMessage{}, err
		}

		if len(resp.Choices) == 0 {
			return ChatMessage{}, fmt.Errorf("empty response from LLM")
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		// If no tool calls, this is a final text response
		if len(assistantMsg.ToolCalls) == 0 {
			respTime := time.Now()
			a.session.Records = append(a.session.Records, memory.Record{
				Timestamp: respTime,
				Role:      "assistant",
				Content:   assistantMsg.Content,
				Tier:      memory.TierHot,
			})
			a.session.UpdatedAt = respTime

			// Generate session title from first message
			a.generateTitleIfNeeded()

			// Check if hot memory exceeds token limit and summarize if needed
			a.compactMemoryIfNeeded()

			// Extract important facts from the latest exchange
			a.extractPinnedMemories()

			a.autoSave()

			// Emit the full response as tokens for the frontend
			wailsRuntime.EventsEmit(a.ctx, "chat:token", assistantMsg.Content)
			wailsRuntime.EventsEmit(a.ctx, "chat:done", nil)

			return ChatMessage{
				Role:      "assistant",
				Content:   assistantMsg.Content,
				Timestamp: respTime.Format("15:04:05"),
			}, nil
		}

		// LLM requested tool calls — add assistant message to history
		a.session.Records = append(a.session.Records, memory.Record{
			Timestamp: time.Now(),
			Role:      "assistant",
			Content:   assistantMsg.Content,
			Tier:      memory.TierHot,
		})

		// Process each tool call
		for _, tc := range assistantMsg.ToolCalls {
			result, err := a.handleToolCall(tc)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}

			wailsRuntime.EventsEmit(a.ctx, "chat:toolresult", map[string]string{
				"name":   tc.Function.Name,
				"result": result,
			})

			// Add tool result to memory as system message so LLM can see it
			a.session.Records = append(a.session.Records, memory.Record{
				Timestamp: time.Now(),
				Role:      "user",
				Content:   fmt.Sprintf("[Tool executed: %s]\nOutput:\n%s", tc.Function.Name, result),
				Tier:      memory.TierHot,
			})
		}

		// Add instruction to respond based on tool results
		a.session.Records = append(a.session.Records, memory.Record{
			Timestamp: time.Now(),
			Role:      "system",
			Content:   "The tool has been executed and the result is shown above. Now respond to the user based on the tool output. Do NOT call any more tools. Provide your answer in natural language.",
			Tier:      memory.TierHot,
		})

		a.autoSave()
		toolsExecuted = true

		// Notify frontend that we're going back to the LLM
		wailsRuntime.EventsEmit(a.ctx, "chat:thinking", nil)
	}

	return ChatMessage{
		Role:      "assistant",
		Content:   "Maximum tool call iterations reached.",
		Timestamp: time.Now().Format("15:04:05"),
	}, nil
}

// handleToolCall executes a single tool call, requesting MITL approval if needed.
func (a *App) handleToolCall(tc client.ToolCall) (string, error) {
	tool, ok := a.tools.Get(tc.Function.Name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", tc.Function.Name)
	}

	req := ToolCallRequest{
		ID:        tc.ID,
		Name:      tc.Function.Name,
		Arguments: tc.Function.Arguments,
		Category:  string(tool.Category),
		NeedsMITL: tool.Category.NeedsMITL(),
	}

	wailsRuntime.EventsEmit(a.ctx, "chat:toolcall_request", req)

	if req.NeedsMITL {
		resp := <-a.mitlCh
		if !resp.approved {
			return fmt.Sprintf("Tool call '%s' was rejected by the user.", tc.Function.Name), nil
		}
	}

	return a.tools.Execute(tc.Function.Name, tc.Function.Arguments)
}

// ApproveMITL is called from the frontend when user approves a tool call.
func (a *App) ApproveMITL() {
	select {
	case a.mitlCh <- mitlResponse{approved: true}:
	default:
	}
}

// RejectMITL is called from the frontend when user rejects a tool call.
func (a *App) RejectMITL() {
	select {
	case a.mitlCh <- mitlResponse{approved: false}:
	default:
	}
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

// RefreshTools rescans the tool script directory.
func (a *App) RefreshTools() ([]ToolInfo, error) {
	if err := a.tools.Scan(); err != nil {
		return nil, err
	}
	return a.GetTools(), nil
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

// GetPinnedMemories returns all pinned memories.
func (a *App) GetPinnedMemories() []memory.PinnedMemory {
	if a.pinned == nil {
		return nil
	}
	return a.pinned.Entries
}

// GetConfig returns the current config for the settings UI.
func (a *App) GetConfig() *config.Config {
	return a.cfg
}

// SaveConfig saves updated config from the settings UI.
func (a *App) SaveConfig(cfgJSON string) error {
	var cfg config.Config
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return err
	}
	cfg.Window = a.cfg.Window
	a.cfg = &cfg
	a.llm = client.New(cfg.API.Endpoint, cfg.API.Model, cfg.API.APIKey)
	a.tools = toolcall.NewRegistry(cfg.Tools.ScriptDir)
	_ = a.tools.Scan()
	return cfg.Save()
}

// generateTitleIfNeeded generates a session title from the first exchange.
func (a *App) generateTitleIfNeeded() {
	if a.session.Title != "New Chat" {
		return
	}

	// Need at least one user message
	var firstUser string
	for _, r := range a.session.Records {
		if r.Role == "user" && r.Tier == memory.TierHot {
			firstUser = r.Content
			break
		}
	}
	if firstUser == "" {
		return
	}

	prompt := []client.Message{
		client.TextMessage("system", "Generate a very short title (under 30 chars) for a chat that starts with the following message. Reply with ONLY the title, no quotes, no explanation. Use the same language as the message."),
		client.TextMessage("user", firstUser),
	}

	resp, err := a.llm.Chat(prompt, nil)
	if err != nil {
		return
	}
	if len(resp.Choices) > 0 {
		title := strings.TrimSpace(resp.Choices[0].Message.Content)
		if title != "" && len(title) < 60 {
			a.session.Title = title
			wailsRuntime.EventsEmit(a.ctx, "chat:title_updated", title)
		}
	}
}

// compactMemoryIfNeeded summarizes old hot records into warm when token limit is exceeded.
func (a *App) compactMemoryIfNeeded() {
	hotTokens := a.session.HotTokenCount()
	limit := a.cfg.Memory.HotTokenLimit
	if hotTokens <= limit {
		return
	}

	excess := hotTokens - (limit * 3 / 4) // compact to 75% of limit
	toSummarize := a.session.PromoteOldestHotToWarm(excess)
	if len(toSummarize) == 0 {
		return
	}

	// Build summarization prompt
	var content string
	for _, r := range toSummarize {
		content += fmt.Sprintf("[%s %s] %s\n", r.Timestamp.Format("15:04:05"), r.Role, r.Content)
	}

	summaryPrompt := []client.Message{
		client.TextMessage("system", "Summarize the following conversation concisely, preserving key facts, decisions, and time references. Write in the same language as the conversation."),
		client.TextMessage("user", content),
	}

	resp, err := a.llm.Chat(summaryPrompt, nil)
	if err != nil {
		fmt.Printf("memory summarization error: %v\n", err)
		return
	}

	if len(resp.Choices) > 0 {
		summary := resp.Choices[0].Message.Content
		a.session.ApplySummary(toSummarize, summary)
		wailsRuntime.EventsEmit(a.ctx, "chat:memory_compacted", map[string]any{
			"summarized_count": len(toSummarize),
			"hot_tokens":       a.session.HotTokenCount(),
		})
	}
}

// extractPinnedMemories asks the LLM to identify important facts from recent messages.
func (a *App) extractPinnedMemories() {
	if a.pinned == nil {
		return
	}

	// Get the last few hot messages for analysis
	hot := a.session.HotRecords()
	if len(hot) < 2 {
		return
	}

	// Only analyze the latest exchange (last 4 messages max)
	start := len(hot) - 4
	if start < 0 {
		start = 0
	}
	recent := hot[start:]

	var conversation string
	for _, r := range recent {
		conversation += fmt.Sprintf("[%s] %s: %s\n", r.Timestamp.Format("15:04:05"), r.Role, r.Content)
	}

	existing := a.pinned.FormatForPrompt()
	prompt := []client.Message{
		client.TextMessage("system", `Analyze the conversation below and extract important facts worth remembering long-term.
Categories: preference, decision, fact, context
Rules:
- Only extract genuinely important, reusable information
- Skip greetings, small talk, and transient details
- If nothing is important, respond with exactly: NONE
- Otherwise respond with one fact per line in format: category|fact
- Do not repeat facts already known
`+"Already known:\n"+existing),
		client.TextMessage("user", conversation),
	}

	resp, err := a.llm.Chat(prompt, nil)
	if err != nil {
		return
	}

	if len(resp.Choices) == 0 {
		return
	}

	text := resp.Choices[0].Message.Content
	if text == "NONE" || text == "" {
		return
	}

	now := time.Now()
	changed := false
	for _, line := range splitLines(text) {
		parts := splitFirst(line, "|")
		if len(parts) != 2 {
			continue
		}
		category := parts[0]
		fact := parts[1]
		if fact == "" {
			continue
		}
		if a.pinned.Add(memory.PinnedMemory{
			Fact:       fact,
			Category:   category,
			SourceTime: now,
			CreatedAt:  now,
		}) {
			changed = true
		}
	}

	if changed {
		_ = a.pinned.Save()
		wailsRuntime.EventsEmit(a.ctx, "chat:pinned_updated", a.pinned.Entries)
	}
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitFirst(s, sep string) []string {
	i := strings.Index(s, sep)
	if i < 0 {
		return []string{s}
	}
	return []string{strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+len(sep):])}
}

func (a *App) autoSave() {
	if a.store != nil && a.session != nil {
		_ = a.store.Save(a.session)
	}
}

func (a *App) buildMessages(systemPrompt string) []client.Message {
	now := time.Now()
	timeContext := fmt.Sprintf(
		"Current date and time: %s\nTimezone: %s",
		now.Format("2006-01-02 15:04:05"),
		now.Location().String(),
	)
	pinnedContext := ""
	if a.pinned != nil {
		if p := a.pinned.FormatForPrompt(); p != "" {
			pinnedContext = "\n\nImportant facts you remember about the user:\n" + p
		}
	}
	fullSystem := fmt.Sprintf("%s\n\n%s%s", systemPrompt, timeContext, pinnedContext)

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
