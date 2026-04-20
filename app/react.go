package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nlink-jp/nlk/jsonfix"
	"github.com/nlink-jp/nlk/strip"
	"github.com/nlink-jp/shell-agent/internal/client"
	"github.com/nlink-jp/shell-agent/internal/logger"
	"github.com/nlink-jp/shell-agent/internal/memory"
	"github.com/nlink-jp/shell-agent/internal/objstore"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// agentLoop runs a simple tool-calling feedback loop.
// LLM is called with tools → if tool calls, execute → feed back → repeat.
// Loop ends when LLM returns text (no tool calls) or max rounds reached.
func (a *App) agentLoop(ctx context.Context, systemPrompt string, toolDefs []client.Tool) (ChatMessage, error) {
	al := logger.New("agent")

	// Reset phase display for new turn
	wailsRuntime.EventsEmit(a.ctx, "chat:phase", nil)

	al.Debug("════════════════════════════════════════")
	al.Debug("=== NEW TURN === tools=%d", len(toolDefs))
	for _, t := range toolDefs {
		al.Debug("  tool: %s", t.Function.Name)
	}

	maxRounds := a.cfg.Memory.MaxToolRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}

	for round := 0; round < maxRounds; round++ {
		if ctx.Err() != nil {
			return ChatMessage{
				Role: "assistant", Content: "(Cancelled)",
				Timestamp: time.Now().Format("15:04:05"),
			}, nil
		}

		messages := a.buildMessages(systemPrompt)
		al.Debug("[ROUND %d] messages=%d tools=%d", round, len(messages), len(toolDefs))

		resp, err := a.llm.ChatWithContext(ctx, messages, toolDefs)
		if err != nil {
			if ctx.Err() != nil {
				return ChatMessage{
					Role: "assistant", Content: "(Cancelled)",
					Timestamp: time.Now().Format("15:04:05"),
				}, nil
			}
			al.Debug("[ROUND %d] error: %v", round, err)
			return ChatMessage{}, err
		}

		// Track tokens (mutex-guarded for concurrent reads from Wails bindings)
		a.addTokenUsage(resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

		if len(resp.Choices) == 0 {
			al.Debug("[ROUND %d] empty choices", round)
			continue
		}

		choice := resp.Choices[0]
		content := strip.ThinkTags(choice.Message.Content)
		content = stripLeakedTimestamps(content)

		al.Debug("[ROUND %d] tool_calls=%d content_len=%d", round, len(choice.Message.ToolCalls), len(content))

		// Check for gemma-style text-based tool calls
		if len(choice.Message.ToolCalls) == 0 {
			if parsed := parseGemmaToolCalls(content); len(parsed) > 0 {
				al.Debug("[ROUND %d] gemma tags: %d tool calls", round, len(parsed))
				for _, tc := range parsed {
					al.Debug("[ROUND %d]   gemma: %s(%s)", round, tc.Function.Name, truncate(tc.Function.Arguments, 100))
				}
				choice.Message.ToolCalls = parsed
				content = stripGemmaToolCallTags(content)
			}
		} else {
			for _, tc := range choice.Message.ToolCalls {
				al.Debug("[ROUND %d]   API: %s(%s)", round, tc.Function.Name, truncate(tc.Function.Arguments, 100))
			}
		}

		content = stripFakeToolCalls(content)
		content = a.resolveLocalImages(content)

		// No tool calls → final text response
		if len(choice.Message.ToolCalls) == 0 {
			content = strings.TrimSpace(content)
			if content == "" {
				al.Debug("[ROUND %d] empty text, continuing", round)
				continue
			}

			al.Debug("[ROUND %d] final text response (%d chars)", round, len(content))
			respTime := time.Now()
			lastIn, lastOut := a.lastTokenUsage()
			a.sessionMu.Lock()
			a.session.Records = append(a.session.Records, memory.Record{
				Timestamp: respTime,
				Role:      "assistant",
				Content:   content,
				Tier:      memory.TierHot,
				InTokens:  lastIn,
				OutTokens: lastOut,
			})
			a.session.UpdatedAt = respTime
			a.sessionMu.Unlock()

			return ChatMessage{
				Role:      "assistant",
				Content:   content,
				Timestamp: respTime.Format("15:04:05"),
				InTokens:  lastIn,
				OutTokens: lastOut,
			}, nil
		}

		// Tool calls detected — store assistant message if it has text
		if strings.TrimSpace(content) != "" {
			a.sessionMu.Lock()
			a.session.Records = append(a.session.Records, memory.Record{
				Timestamp: time.Now(),
				Role:      "assistant",
				Content:   content,
				Tier:      memory.TierHot,
			})
			a.sessionMu.Unlock()
		}

		// Execute each tool call
		al.Debug("[ROUND %d] executing %d tool calls", round, len(choice.Message.ToolCalls))
		wailsRuntime.EventsEmit(a.ctx, "chat:phase", fmt.Sprintf("execute (round %d)", round+1))

		for _, tc := range choice.Message.ToolCalls {
			al.Debug("[ROUND %d]   calling: %s", round, tc.Function.Name)
			wailsRuntime.EventsEmit(a.ctx, "chat:tool_executing", map[string]string{
				"name": tc.Function.Name,
				"args": tc.Function.Arguments,
			})
			result, tcErr := a.handleToolCall(tc)
			if tcErr != nil {
				al.Debug("[ROUND %d]   error: %v", round, tcErr)
				result = fmt.Sprintf("Error: %v", tcErr)
			} else {
				al.Debug("[ROUND %d]   result: %s", round, truncate(result, 200))
			}

			// Save image to ImageStore and emit ID (not data URL)
			var toolImages []memory.ImageEntry
			imageDataURL := a.extractImageFromResult(result)
			if imageDataURL == "" && strings.Contains(result, "[Artifacts produced:") {
				if idx := strings.Index(result, "[Artifacts produced:"); idx >= 0 {
					artStr := result[idx:]
					if end := strings.Index(artStr, "]"); end >= 0 {
						refs := strings.Fields(artStr[len("[Artifacts produced:"):end])
						for _, ref := range refs {
							if a.objects != nil {
								if du, loadErr := a.objects.LoadAsDataURL(ref); loadErr == nil {
									imageDataURL = du
									break
								}
							}
						}
					}
				}
			}
			var imageStoreID string
			if imageDataURL != "" && a.objects != nil {
				id, saveErr := a.objects.SaveDataURL(imageDataURL, objstore.TypeImage, "")
				if saveErr == nil {
					toolImages = append(toolImages, memory.ImageEntry{ID: id})
					imageStoreID = id
				}
			}

			toolResultEvent := map[string]string{
				"name":   tc.Function.Name,
				"result": result,
			}
			if imageStoreID != "" {
				toolResultEvent["imageId"] = imageStoreID
			}
			wailsRuntime.EventsEmit(a.ctx, "chat:toolresult", toolResultEvent)

			a.sessionMu.Lock()
			a.session.Records = append(a.session.Records, memory.Record{
				Timestamp: time.Now(),
				Role:      "tool",
				Content:   fmt.Sprintf("[Tool executed: %s]\nOutput:\n%s", tc.Function.Name, result),
				Tier:      memory.TierHot,
				Images:    toolImages,
			})
			a.sessionMu.Unlock()
		}

		a.autoSave()
		wailsRuntime.EventsEmit(a.ctx, "chat:thinking", nil)
	}

	al.Debug("max rounds reached")
	wailsRuntime.EventsEmit(a.ctx, "chat:phase", nil)

	return ChatMessage{
		Role:      "assistant",
		Content:   "Maximum tool call iterations reached.",
		Timestamp: time.Now().Format("15:04:05"),
	}, nil
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

// parseGemmaToolCalls extracts tool calls from gemma-style text tags.
// Format: <|tool_call>call:tool_name{json_args}<tool_call|>
func parseGemmaToolCalls(text string) []client.ToolCall {
	var calls []client.ToolCall

	remaining := text
	for {
		start := strings.Index(remaining, "<|tool_call>")
		if start < 0 {
			start = strings.Index(remaining, "<tool_call>")
			if start < 0 {
				break
			}
			remaining = remaining[start+len("<tool_call>"):]
		} else {
			remaining = remaining[start+len("<|tool_call>"):]
		}

		end := strings.Index(remaining, "<tool_call|>")
		if end < 0 {
			end = strings.Index(remaining, "</tool_call>")
			if end < 0 {
				break
			}
		}

		callStr := strings.TrimSpace(remaining[:end])
		remaining = remaining[end:]

		if !strings.HasPrefix(callStr, "call:") {
			continue
		}
		callStr = strings.TrimPrefix(callStr, "call:")

		var toolName, argsStr string
		braceIdx := strings.Index(callStr, "{")
		if braceIdx >= 0 {
			toolName = strings.TrimSpace(callStr[:braceIdx])
			argsStr = callStr[braceIdx:]
		} else {
			toolName = strings.TrimSpace(callStr)
			argsStr = "{}"
		}

		toolName = strings.Trim(toolName, "\" '")
		if toolName == "" {
			continue
		}

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
