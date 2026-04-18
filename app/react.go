package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nlink-jp/nlk/jsonfix"
	"github.com/nlink-jp/nlk/strip"
	"github.com/nlink-jp/shell-agent/internal/client"
	"github.com/nlink-jp/shell-agent/internal/memory"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Plan represents a structured execution plan from the planning phase.
type Plan struct {
	Goal  string     `json:"goal"`
	Steps []PlanStep `json:"steps"`
}

// PlanStep is one step in the plan.
type PlanStep struct {
	Step        int    `json:"step"`
	Description string `json:"description"`
	Tool        string `json:"tool"`
	Reason      string `json:"reason"`
}

// reactLoop orchestrates the Plan → Execute → Summarize phases.
func (a *App) reactLoop(ctx context.Context, systemPrompt string, toolDefs []client.Tool) (ChatMessage, error) {
	// Phase 1: Plan
	var plan *Plan
	if len(toolDefs) > 0 {
		wailsRuntime.EventsEmit(a.ctx, "chat:phase", "plan")
		var err error
		plan, err = a.planPhase(ctx, toolDefs)
		if err != nil {
			// Planning failed — proceed without plan
			fmt.Printf("plan phase error: %v\n", err)
		} else {
			wailsRuntime.EventsEmit(a.ctx, "chat:plan", plan)
		}
	}

	// Phase 2: Execute
	wailsRuntime.EventsEmit(a.ctx, "chat:phase", "execute")
	toolsUsed, err := a.executePhase(ctx, systemPrompt, plan, toolDefs)
	if err != nil {
		if ctx.Err() != nil {
			return ChatMessage{
				Role: "assistant", Content: "(Cancelled)",
				Timestamp: time.Now().Format("15:04:05"),
			}, nil
		}
		return ChatMessage{}, err
	}

	// Phase 3: Summarize (only if tools were used)
	if toolsUsed {
		wailsRuntime.EventsEmit(a.ctx, "chat:phase", "summarize")
		content, err := a.summarizePhase(ctx, systemPrompt)
		if err != nil {
			if ctx.Err() != nil {
				return ChatMessage{
					Role: "assistant", Content: "(Cancelled)",
					Timestamp: time.Now().Format("15:04:05"),
				}, nil
			}
			return ChatMessage{}, err
		}

		// If summarize returned empty, use the last interim summary
		if content == "" {
			content = a.lastInterimSummary()
		}

		if content != "" {
			respTime := time.Now()
			a.session.Records = append(a.session.Records, memory.Record{
				Timestamp: respTime,
				Role:      "assistant",
				Content:   content,
				Tier:      memory.TierHot,
				InTokens:  a.tokenStats.LastInput,
				OutTokens: a.tokenStats.LastOutput,
			})
			a.session.UpdatedAt = respTime
		}

		wailsRuntime.EventsEmit(a.ctx, "chat:phase", nil)

		return ChatMessage{
			Role:      "assistant",
			Content:   content,
			Timestamp: time.Now().Format("15:04:05"),
			InTokens:  a.tokenStats.LastInput,
			OutTokens: a.tokenStats.LastOutput,
		}, nil
	}

	// No tools used — the execute phase already stored the final text response
	wailsRuntime.EventsEmit(a.ctx, "chat:phase", nil)

	// Find the last assistant record
	for i := len(a.session.Records) - 1; i >= 0; i-- {
		r := a.session.Records[i]
		if r.Role == "assistant" && r.Tier == memory.TierHot {
			return ChatMessage{
				Role:      "assistant",
				Content:   r.Content,
				Timestamp: r.Timestamp.Format("15:04:05"),
				InTokens:  r.InTokens,
				OutTokens: r.OutTokens,
			}, nil
		}
	}

	return ChatMessage{
		Role: "assistant", Content: "",
		Timestamp: time.Now().Format("15:04:05"),
	}, nil
}

// planPhase generates a structured execution plan.
func (a *App) planPhase(ctx context.Context, toolDefs []client.Tool) (*Plan, error) {
	// Build tool list description
	var toolDescs []string
	for _, t := range toolDefs {
		toolDescs = append(toolDescs, fmt.Sprintf("- %s: %s", t.Function.Name, t.Function.Description))
	}

	// Get the last user message
	var userMsg string
	for i := len(a.session.Records) - 1; i >= 0; i-- {
		if a.session.Records[i].Role == "user" && a.session.Records[i].Tier == memory.TierHot {
			userMsg = a.session.Records[i].Content
			break
		}
	}

	prompt := []client.Message{
		client.TextMessage("system", `Given the user's request and available tools, create a brief execution plan.
Respond with ONLY valid JSON, no other text:
{"goal": "brief goal description", "steps": [{"step": 1, "description": "what to do", "tool": "tool_name or null", "reason": "why"}]}

If no tools are needed (simple question/conversation), respond:
{"goal": "answer directly", "steps": [{"step": 1, "description": "respond to user", "tool": null, "reason": "no tools needed"}]}

Available tools:
`+strings.Join(toolDescs, "\n")),
		client.TextMessage("user", userMsg),
	}

	resp, err := a.llm.ChatWithContext(ctx, prompt, nil)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	return parsePlan(resp.Choices[0].Message.Content, userMsg)
}

// parsePlan extracts a Plan from LLM output, with fallback.
func parsePlan(text, userGoal string) (*Plan, error) {
	text = strings.TrimSpace(text)

	// Try to find JSON in the response
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		jsonStr := text[start : end+1]
		var plan Plan
		if err := json.Unmarshal([]byte(jsonStr), &plan); err == nil && len(plan.Steps) > 0 {
			return &plan, nil
		}
	}

	// Fallback: single step plan
	return &Plan{
		Goal: userGoal,
		Steps: []PlanStep{
			{Step: 1, Description: "Respond to user request", Tool: "", Reason: "fallback plan"},
		},
	}, nil
}

// planHasMoreToolSteps checks if the plan has remaining tool steps after the given round.
func planHasMoreToolSteps(plan *Plan, completedRounds int) bool {
	if plan == nil {
		return false
	}
	toolStepCount := 0
	for _, s := range plan.Steps {
		if s.Tool != "" && s.Tool != "null" {
			toolStepCount++
		}
	}
	return completedRounds < toolStepCount
}

// planNeedsTools returns true if the plan suggests using any tools.
func planNeedsTools(plan *Plan) bool {
	if plan == nil {
		return false
	}
	for _, s := range plan.Steps {
		if s.Tool != "" && s.Tool != "null" {
			return true
		}
	}
	return false
}

// executePhase runs the ReAct loop. Returns true if any tools were used.
func (a *App) executePhase(ctx context.Context, systemPrompt string, plan *Plan, toolDefs []client.Tool) (bool, error) {
	maxRounds := a.cfg.Memory.MaxToolRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}

	// Inject plan hints into system prompt if plan exists and suggests tools
	execPrompt := systemPrompt
	if planNeedsTools(plan) {
		var hints []string
		for _, s := range plan.Steps {
			if s.Tool != "" && s.Tool != "null" {
				hints = append(hints, fmt.Sprintf("Step %d: %s (tool: %s)", s.Step, s.Description, s.Tool))
			}
		}
		execPrompt += "\n\nSuggested plan:\n" + strings.Join(hints, "\n") +
			"\nFollow this plan by calling the appropriate tools. Do NOT describe what you will do — call the tool directly."
	}

	toolsUsed := false

	for round := 0; round < maxRounds; round++ {
		if ctx.Err() != nil {
			return toolsUsed, ctx.Err()
		}

		messages := a.buildMessagesWithPrompt(execPrompt)

		// Determine whether to include tools
		var currentTools []client.Tool
		if round < maxRounds {
			currentTools = toolDefs
		}

		resp, err := a.llm.ChatWithContext(ctx, messages, currentTools)
		if err != nil {
			return toolsUsed, err
		}

		// Track tokens
		a.tokenStats.LastInput = resp.Usage.PromptTokens
		a.tokenStats.LastOutput = resp.Usage.CompletionTokens
		a.tokenStats.TotalInput += resp.Usage.PromptTokens
		a.tokenStats.TotalOutput += resp.Usage.CompletionTokens

		if len(resp.Choices) == 0 {
			return toolsUsed, fmt.Errorf("empty response")
		}

		choice := resp.Choices[0]
		assistantContent := strip.ThinkTags(choice.Message.Content)
		assistantContent = stripLeakedTimestamps(assistantContent)

		// Check for gemma-style text-based tool calls: <|tool_call>call:name{args}<tool_call|>
		if len(choice.Message.ToolCalls) == 0 {
			if parsed := parseGemmaToolCalls(assistantContent); len(parsed) > 0 {
				choice.Message.ToolCalls = parsed
				// Remove tool call tags from displayed text
				assistantContent = stripGemmaToolCallTags(assistantContent)
			}
		}

		assistantContent = stripFakeToolCalls(assistantContent)
		assistantContent = a.resolveLocalImages(assistantContent)

		// No tool calls — this is a text response
		if len(choice.Message.ToolCalls) == 0 {
			content := strings.TrimSpace(assistantContent)
			if content == "" {
				continue
			}

			respTime := time.Now()
			a.session.Records = append(a.session.Records, memory.Record{
				Timestamp: respTime,
				Role:      "assistant",
				Content:   content,
				Tier:      memory.TierHot,
				InTokens:  a.tokenStats.LastInput,
				OutTokens: a.tokenStats.LastOutput,
			})
			a.session.UpdatedAt = respTime

			return toolsUsed, nil
		}

		// Tool calls detected
		toolsUsed = true

		// Store assistant message (may contain text before tool calls)
		if assistantContent != "" {
			a.session.Records = append(a.session.Records, memory.Record{
				Timestamp: time.Now(),
				Role:      "assistant",
				Content:   assistantContent,
				Tier:      memory.TierHot,
			})
		}

		// Execute each tool call
		wailsRuntime.EventsEmit(a.ctx, "chat:phase", fmt.Sprintf("execute (step %d)", round+1))
		for _, tc := range choice.Message.ToolCalls {
			result, err := a.handleToolCall(tc)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}

			// Extract image and emit tool result
			imageDataURL := a.extractImageFromResult(result)
			toolResultEvent := map[string]string{
				"name":   tc.Function.Name,
				"result": result,
			}
			if imageDataURL != "" {
				toolResultEvent["image"] = imageDataURL
			} else if strings.Contains(result, "[Artifacts produced:") {
				if idx := strings.Index(result, "[Artifacts produced:"); idx >= 0 {
					artStr := result[idx:]
					if end := strings.Index(artStr, "]"); end >= 0 {
						refs := strings.Fields(artStr[len("[Artifacts produced:"):end])
						for _, ref := range refs {
							if a.jobs != nil {
								blobPath := a.jobs.BlobPath(ref)
								if du := a.fileToDataURL(blobPath); du != "" {
									toolResultEvent["image"] = du
									break
								}
							}
						}
					}
				}
			}
			wailsRuntime.EventsEmit(a.ctx, "chat:toolresult", toolResultEvent)

			// Save tool result with images
			var toolImages []memory.ImageEntry
			if imgURL, ok := toolResultEvent["image"]; ok && imgURL != "" && a.images != nil {
				id, mime, saveErr := a.images.Save(imgURL)
				if saveErr == nil {
					toolImages = append(toolImages, memory.ImageEntry{ID: id, MimeType: mime})
				}
			}

			a.session.Records = append(a.session.Records, memory.Record{
				Timestamp: time.Now(),
				Role:      "tool",
				Content:   fmt.Sprintf("[Tool executed: %s]\nOutput:\n%s", tc.Function.Name, result),
				Tier:      memory.TierHot,
				Images:    toolImages,
			})
		}

		a.autoSave()

		// Interim summary: force text-only call to let LLM observe results
		if ctx.Err() != nil {
			return toolsUsed, ctx.Err()
		}

		wailsRuntime.EventsEmit(a.ctx, "chat:thinking", nil)

		interimMessages := a.buildMessagesWithPrompt(systemPrompt) // use base prompt without tool hints
		// Add instruction for interim summary
		interimMessages = append(interimMessages, client.TextMessage("system",
			"The tool(s) have been executed and the results are shown above. "+
				"Briefly describe what you learned. Then decide: do you need another tool to complete the task, or can you give the final answer?"))

		interimResp, err := a.llm.ChatWithContext(ctx, interimMessages, nil) // no tools = force text
		if err != nil {
			return toolsUsed, err
		}

		a.tokenStats.LastInput = interimResp.Usage.PromptTokens
		a.tokenStats.LastOutput = interimResp.Usage.CompletionTokens
		a.tokenStats.TotalInput += interimResp.Usage.PromptTokens
		a.tokenStats.TotalOutput += interimResp.Usage.CompletionTokens

		continueLoop := false
		if len(interimResp.Choices) > 0 {
			interim := strings.TrimSpace(strip.ThinkTags(interimResp.Choices[0].Message.Content))
			interim = stripLeakedTimestamps(interim)

			// Check if interim contains gemma tool call tags — if so, execute them
			if parsed := parseGemmaToolCalls(interim); len(parsed) > 0 {
				cleanInterim := stripGemmaToolCallTags(interim)
				if cleanInterim != "" {
					a.session.Records = append(a.session.Records, memory.Record{
						Timestamp: time.Now(),
						Role:      "assistant",
						Content:   cleanInterim,
						Tier:      memory.TierHot,
					})
				}
				// Execute the tool calls from interim
				for _, tc := range parsed {
					result, tcErr := a.handleToolCall(tc)
					if tcErr != nil {
						result = fmt.Sprintf("Error: %v", tcErr)
					}
					toolResultEvent := map[string]string{"name": tc.Function.Name, "result": result}
					imageDataURL := a.extractImageFromResult(result)
					if imageDataURL != "" {
						toolResultEvent["image"] = imageDataURL
					}
					wailsRuntime.EventsEmit(a.ctx, "chat:toolresult", toolResultEvent)

					var toolImages []memory.ImageEntry
					if imgURL, ok := toolResultEvent["image"]; ok && imgURL != "" && a.images != nil {
						id, mime, saveErr := a.images.Save(imgURL)
						if saveErr == nil {
							toolImages = append(toolImages, memory.ImageEntry{ID: id, MimeType: mime})
						}
					}
					a.session.Records = append(a.session.Records, memory.Record{
						Timestamp: time.Now(),
						Role:      "tool",
						Content:   fmt.Sprintf("[Tool executed: %s]\nOutput:\n%s", tc.Function.Name, result),
						Tier:      memory.TierHot,
						Images:    toolImages,
					})
				}
				continueLoop = true
			} else if interim != "" {
				a.session.Records = append(a.session.Records, memory.Record{
					Timestamp: time.Now(),
					Role:      "assistant",
					Content:   interim,
					Tier:      memory.TierHot,
				})
				wailsRuntime.EventsEmit(a.ctx, "chat:interim", interim)
			}
		}

		// Clean up ephemeral system messages
		a.cleanEphemeralMessages()

		if !continueLoop {
			break
		}
	}

	return toolsUsed, nil
}

// summarizePhase generates the final response after tool execution.
func (a *App) summarizePhase(ctx context.Context, systemPrompt string) (string, error) {
	messages := a.buildMessagesWithPrompt(systemPrompt)
	messages = append(messages, client.TextMessage("system",
		"Based on all the actions you performed and their results, provide a final concise answer to the user. Do NOT call any more tools."))

	resp, err := a.llm.ChatWithContext(ctx, messages, nil) // no tools
	if err != nil {
		return "", err
	}

	a.tokenStats.LastInput = resp.Usage.PromptTokens
	a.tokenStats.LastOutput = resp.Usage.CompletionTokens
	a.tokenStats.TotalInput += resp.Usage.PromptTokens
	a.tokenStats.TotalOutput += resp.Usage.CompletionTokens

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}

	content := strings.TrimSpace(strip.ThinkTags(resp.Choices[0].Message.Content))
	content = stripLeakedTimestamps(content)
	content = a.resolveLocalImages(content)
	return content, nil
}

// buildMessagesWithPrompt is like buildMessages but uses a custom system prompt.
func (a *App) buildMessagesWithPrompt(systemPrompt string) []client.Message {
	// Save the original prompt builder's output but replace the system prompt
	return a.buildMessagesCustom(systemPrompt)
}

// lastInterimSummary returns the most recent assistant message content (interim summary).
func (a *App) lastInterimSummary() string {
	for i := len(a.session.Records) - 1; i >= 0; i-- {
		r := a.session.Records[i]
		if r.Role == "assistant" && r.Tier == memory.TierHot && r.Content != "" {
			return r.Content
		}
	}
	return ""
}

// parseGemmaToolCalls extracts tool calls from gemma-style text tags.
// Format: <|tool_call>call:tool_name{json_args}<tool_call|>
// Also handles: <|tool_call>call:tool_name {"key": "value"}<tool_call|>
func parseGemmaToolCalls(text string) []client.ToolCall {
	var calls []client.ToolCall

	remaining := text
	for {
		// Find opening tag
		start := strings.Index(remaining, "<|tool_call>")
		if start < 0 {
			// Try alternative format
			start = strings.Index(remaining, "<tool_call>")
			if start < 0 {
				break
			}
			remaining = remaining[start+len("<tool_call>"):]
		} else {
			remaining = remaining[start+len("<|tool_call>"):]
		}

		// Find closing tag
		end := strings.Index(remaining, "<tool_call|>")
		if end < 0 {
			end = strings.Index(remaining, "</tool_call>")
			if end < 0 {
				break
			}
		}

		callStr := strings.TrimSpace(remaining[:end])
		remaining = remaining[end:]

		// Parse: call:tool_name{args} or call:tool_name {"key": "val"}
		if !strings.HasPrefix(callStr, "call:") {
			continue
		}
		callStr = strings.TrimPrefix(callStr, "call:")

		// Find where the tool name ends and args begin
		var toolName, argsStr string
		braceIdx := strings.Index(callStr, "{")
		if braceIdx >= 0 {
			toolName = strings.TrimSpace(callStr[:braceIdx])
			argsStr = callStr[braceIdx:]
		} else {
			// No args
			toolName = strings.TrimSpace(callStr)
			argsStr = "{}"
		}

		// Clean up tool name (remove quotes, spaces)
		toolName = strings.Trim(toolName, "\" '")

		if toolName == "" {
			continue
		}

		// Try to fix common JSON issues in args
		if fixed, err := jsonfix.Extract(argsStr); err == nil {
			argsStr = fixed
		}

		calls = append(calls, client.ToolCall{
			ID:   fmt.Sprintf("gemma-%d", len(calls)),
			Type: "function",
			Function: client.FunctionCall{
				Name:      toolName,
				Arguments: argsStr,
			},
		})
	}

	return calls
}

// stripGemmaToolCallTags removes gemma-style tool call tags from text.
func stripGemmaToolCallTags(text string) string {
	result := text
	for {
		start := strings.Index(result, "<|tool_call>")
		if start < 0 {
			start = strings.Index(result, "<tool_call>")
			if start < 0 {
				break
			}
		}

		end := strings.Index(result[start:], "<tool_call|>")
		endLen := len("<tool_call|>")
		if end < 0 {
			end = strings.Index(result[start:], "</tool_call>")
			endLen = len("</tool_call>")
			if end < 0 {
				break
			}
		}
		result = result[:start] + result[start+end+endLen:]
	}
	return strings.TrimSpace(result)
}

// cleanEphemeralMessages removes turn-local system instruction messages.
func (a *App) cleanEphemeralMessages() {
	filtered := a.session.Records[:0]
	for _, r := range a.session.Records {
		if r.Role == "system" && (strings.Contains(r.Content, "tool") || strings.Contains(r.Content, "Briefly describe")) {
			continue
		}
		filtered = append(filtered, r)
	}
	a.session.Records = filtered
}
