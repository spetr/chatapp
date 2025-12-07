package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/spetr/chatapp/internal/models"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
)

type AnthropicProvider struct {
	apiKey string
	models []string
	client *http.Client
}

func NewAnthropicProvider(apiKey string, modelList []string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey: apiKey,
		models: modelList,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (p *AnthropicProvider) Name() string {
	return "claude"
}

func (p *AnthropicProvider) Models() []string {
	return p.models
}

type anthropicMessage struct {
	Role    string        `json:"role"`
	Content []interface{} `json:"content"`
}

type anthropicTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicImageContent struct {
	Type   string               `json:"type"`
	Source anthropicImageSource `json:"source"`
}

type anthropicToolUseContent struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

type anthropicToolResultContent struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicThinking struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"` // min 1024
}

type anthropicRequest struct {
	Model     string                 `json:"model"`
	MaxTokens int                    `json:"max_tokens"`
	System    []anthropicSystemBlock `json:"system,omitempty"`
	Messages  []anthropicMessage     `json:"messages"`
	Stream    bool                   `json:"stream"`
	Tools     []anthropicTool        `json:"tools,omitempty"`
	Thinking  *anthropicThinking     `json:"thinking,omitempty"`
}

type anthropicSystemBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []models.Message, model string, systemPrompt string, opts *ChatOptions, callback StreamCallback) error {
	return p.ChatWithTools(ctx, messages, model, systemPrompt, nil, opts, callback)
}

func (p *AnthropicProvider) ChatWithTools(ctx context.Context, messages []models.Message, model string, systemPrompt string, tools []Tool, opts *ChatOptions, callback StreamCallback) error {
	startTime := time.Now()
	var ttfb float64
	var outputTokens int

	// Convert messages to Anthropic format
	anthropicMsgs := make([]anthropicMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" {
			continue // System prompt is handled separately
		}

		content := make([]interface{}, 0)

		// Add text content
		if msg.Content != "" {
			content = append(content, anthropicTextContent{
				Type: "text",
				Text: msg.Content,
			})
		}

		// Add attachments (images)
		for _, att := range msg.Attachments {
			if strings.HasPrefix(att.MimeType, "image/") && att.Data != "" {
				content = append(content, anthropicImageContent{
					Type: "image",
					Source: anthropicImageSource{
						Type:      "base64",
						MediaType: att.MimeType,
						Data:      att.Data,
					},
				})
			}
		}

		// Add tool calls (for assistant messages)
		for _, tc := range msg.ToolCalls {
			content = append(content, anthropicToolUseContent{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Name,
				Input: tc.Arguments,
			})
		}

		// Add tool results (for user messages)
		for _, tr := range msg.ToolResults {
			content = append(content, anthropicToolResultContent{
				Type:      "tool_result",
				ToolUseID: tr.ToolUseID,
				Content:   tr.Content,
				IsError:   tr.IsError,
			})
		}

		if len(content) > 0 {
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    msg.Role,
				Content: content,
			})
		}
	}

	// Build request with prompt caching
	var systemBlocks []anthropicSystemBlock
	if systemPrompt != "" {
		systemBlocks = []anthropicSystemBlock{
			{
				Type: "text",
				Text: systemPrompt,
				CacheControl: &anthropicCacheControl{
					Type: "ephemeral",
				},
			},
		}
	}

	// Add cache control to older messages (cache first 80% of conversation)
	cacheBreakpoint := len(anthropicMsgs) * 80 / 100
	if cacheBreakpoint > 0 && len(anthropicMsgs) > 4 {
		// Mark the message at cache breakpoint for caching
		lastContent := anthropicMsgs[cacheBreakpoint-1].Content
		if len(lastContent) > 0 {
			if textContent, ok := lastContent[len(lastContent)-1].(anthropicTextContent); ok {
				anthropicMsgs[cacheBreakpoint-1].Content[len(lastContent)-1] = map[string]interface{}{
					"type":          "text",
					"text":          textContent.Text,
					"cache_control": map[string]string{"type": "ephemeral"},
				}
			}
		}
	}

	// Determine max tokens - need more for extended thinking
	maxTokens := 4096
	enableThinking := false
	if opts != nil && opts.EnableThinking {
		enableThinking = true
		maxTokens = 16000 // Extended thinking needs more output tokens
	}
	if opts != nil && opts.MaxTokens != nil {
		maxTokens = *opts.MaxTokens
	}

	req := anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    systemBlocks,
		Messages:  anthropicMsgs,
		Stream:    true,
	}

	// Add extended thinking if enabled
	if enableThinking {
		budgetTokens := 10000 // default
		if opts != nil && opts.ThinkingBudget != "" {
			// Parse budget from string (could be "low", "medium", "high" or a number)
			switch opts.ThinkingBudget {
			case "low":
				budgetTokens = 5000
			case "medium":
				budgetTokens = 10000
			case "high":
				budgetTokens = 20000
			default:
				// Try to parse as number
				if n, err := fmt.Sscanf(opts.ThinkingBudget, "%d", &budgetTokens); n != 1 || err != nil {
					budgetTokens = 10000
				}
			}
		}
		req.Thinking = &anthropicThinking{
			Type:         "enabled",
			BudgetTokens: budgetTokens,
		}
	}

	// Add tools if provided
	if len(tools) > 0 {
		req.Tools = make([]anthropicTool, len(tools))
		for i, t := range tools {
			req.Tools[i] = anthropicTool(t)
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	// Build beta features list
	betaFeatures := []string{"prompt-caching-2024-07-31"}
	// For interleaved thinking with tool use, add the interleaved thinking beta
	if enableThinking && len(tools) > 0 {
		betaFeatures = append(betaFeatures, "interleaved-thinking-2025-05-14")
	}
	httpReq.Header.Set("anthropic-beta", strings.Join(betaFeatures, ","))

	// Send start event with raw request for debugging
	callback(models.StreamEvent{
		Type: "debug",
		Data: map[string]interface{}{
			"request": map[string]interface{}{
				"url":    anthropicAPIURL,
				"method": "POST",
				"headers": map[string]string{
					"anthropic-version": anthropicAPIVersion,
				},
				"body": req,
			},
		},
	})

	callback(models.StreamEvent{Type: "start"})

	resp, err := p.client.Do(httpReq)
	if err != nil {
		callback(models.StreamEvent{
			Type:  "error",
			Error: fmt.Sprintf("request failed: %v", err),
		})
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("API error %d: %s", resp.StatusCode, string(body))
		callback(models.StreamEvent{
			Type:  "error",
			Error: errMsg,
		})
		return fmt.Errorf("%s", errMsg)
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var inputTokens int
	var cacheCreationTokens int
	var cacheReadTokens int
	firstChunk := true

	// Tool call tracking
	var currentToolID string
	var currentToolName string
	var toolJSONBuffer strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)

		switch eventType {
		case "message_start":
			if msg, ok := event["message"].(map[string]interface{}); ok {
				if usage, ok := msg["usage"].(map[string]interface{}); ok {
					if it, ok := usage["input_tokens"].(float64); ok {
						inputTokens = int(it)
					}
					if cct, ok := usage["cache_creation_input_tokens"].(float64); ok {
						cacheCreationTokens = int(cct)
					}
					if crt, ok := usage["cache_read_input_tokens"].(float64); ok {
						cacheReadTokens = int(crt)
					}
				}
			}

		case "content_block_delta":
			if firstChunk {
				ttfb = float64(time.Since(startTime).Milliseconds())
				firstChunk = false
			}

			if delta, ok := event["delta"].(map[string]interface{}); ok {
				if deltaType, ok := delta["type"].(string); ok {
					switch deltaType {
					case "text_delta":
						if text, ok := delta["text"].(string); ok {
							outputTokens += len(strings.Fields(text)) // Rough estimate
							callback(models.StreamEvent{
								Type:    "delta",
								Content: text,
							})
						}
					case "thinking_delta":
						// Extended thinking content
						if thinking, ok := delta["thinking"].(string); ok {
							callback(models.StreamEvent{
								Type:    "thinking",
								Content: thinking,
							})
						}
					case "input_json_delta":
						// Tool use delta - accumulate JSON fragments
						if partialJSON, ok := delta["partial_json"].(string); ok {
							toolJSONBuffer.WriteString(partialJSON)
							callback(models.StreamEvent{
								Type: "tool_delta",
								Data: map[string]interface{}{
									"partial_json": partialJSON,
								},
							})
						}
					}
				}
			}

		case "content_block_start":
			if cb, ok := event["content_block"].(map[string]interface{}); ok {
				cbType, _ := cb["type"].(string)
				switch cbType {
				case "tool_use":
					currentToolID, _ = cb["id"].(string)
					currentToolName, _ = cb["name"].(string)
					// Fallback: generate unique ID if server doesn't provide one
					if currentToolID == "" {
						currentToolID = fmt.Sprintf("call_%d", time.Now().UnixNano())
					}
					toolJSONBuffer.Reset()
					callback(models.StreamEvent{
						Type: "tool_start",
						Data: map[string]interface{}{
							"id":   currentToolID,
							"name": currentToolName,
						},
					})
				case "thinking":
					// Extended thinking block started - we'll get thinking_delta events
					// No need to emit anything here, just track it
				}
			}

		case "content_block_stop":
			// Content block finished - if we were building a tool call, emit completion
			if currentToolID != "" {
				var arguments map[string]interface{}
				jsonStr := toolJSONBuffer.String()
				if jsonStr != "" {
					if err := json.Unmarshal([]byte(jsonStr), &arguments); err != nil {
						log.Printf("Failed to parse tool arguments JSON: %v", err)
					}
				}
				callback(models.StreamEvent{
					Type: "tool_complete",
					Data: map[string]interface{}{
						"id":        currentToolID,
						"name":      currentToolName,
						"arguments": arguments,
					},
				})
				currentToolID = ""
				currentToolName = ""
				toolJSONBuffer.Reset()
			}

		case "message_delta":
			if usage, ok := event["usage"].(map[string]interface{}); ok {
				if ot, ok := usage["output_tokens"].(float64); ok {
					outputTokens = int(ot)
				}
			}

		case "message_stop":
			// Message complete
		}
	}

	totalLatency := float64(time.Since(startTime).Milliseconds())
	tokensPerSec := 0.0
	if totalLatency > ttfb && outputTokens > 0 {
		tokensPerSec = float64(outputTokens) / ((totalLatency - ttfb) / 1000)
	}

	callback(models.StreamEvent{
		Type: "metrics",
		Metrics: &models.Metrics{
			InputTokens:         inputTokens,
			OutputTokens:        outputTokens,
			TotalTokens:         inputTokens + outputTokens,
			CacheCreationTokens: cacheCreationTokens,
			CacheReadTokens:     cacheReadTokens,
			TimeToFirstByte:     ttfb,
			TotalLatency:        totalLatency,
			TokensPerSecond:     tokensPerSec,
		},
	})

	callback(models.StreamEvent{Type: "done"})

	return nil
}

func (p *AnthropicProvider) CountTokens(messages []models.Message) (int, error) {
	// Rough estimation: ~4 chars per token for English
	total := 0
	for _, msg := range messages {
		total += len(msg.Content) / 4
	}
	return total, nil
}
