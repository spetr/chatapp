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
	openaiAPIURL = "https://api.openai.com/v1/chat/completions"
)

type OpenAIProvider struct {
	apiKey  string
	baseURL string
	models  []string
	client  *http.Client
}

func NewOpenAIProvider(apiKey string, modelList []string, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = openaiAPIURL
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		models:  modelList,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

func (p *OpenAIProvider) Models() []string {
	return p.models
}

type openaiMessage struct {
	Role       string                   `json:"role"`
	Content    interface{}              `json:"content"`               // string or []openaiContentPart
	ToolCalls  []openaiMessageToolCall  `json:"tool_calls,omitempty"`  // For assistant messages with tool calls
	ToolCallID string                   `json:"tool_call_id,omitempty"` // For tool result messages
}

type openaiMessageToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openaiImageURL `json:"image_url,omitempty"`
}

type openaiImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type openaiRequest struct {
	Model               string               `json:"model"`
	Messages            []openaiMessage      `json:"messages"`
	MaxCompletionTokens int                  `json:"max_completion_tokens,omitempty"`
	Stream              bool                 `json:"stream"`
	StreamOptions       *openaiStreamOptions `json:"stream_options,omitempty"`
	Temperature         *float64             `json:"temperature,omitempty"`
	Tools               []openaiTool         `json:"tools,omitempty"`
	ReasoningEffort     string               `json:"reasoning_effort,omitempty"` // low/medium/high for o-series
}

// isReasoningModel checks if the model is an o-series reasoning model
func isReasoningModel(model string) bool {
	modelLower := strings.ToLower(model)
	// o1, o1-mini, o1-preview, o3, o3-mini, o3-pro, o4-mini
	if strings.HasPrefix(modelLower, "o1") ||
		strings.HasPrefix(modelLower, "o3") ||
		strings.HasPrefix(modelLower, "o4") {
		return true
	}
	return false
}

type openaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openaiTool struct {
	Type     string             `json:"type"`
	Function openaiToolFunction `json:"function"`
}

type openaiToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openaiToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type openaiStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string                `json:"role,omitempty"`
			Content   string                `json:"content,omitempty"`
			ToolCalls []openaiToolCallDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []models.Message, model string, systemPrompt string, opts *ChatOptions, callback StreamCallback) error {
	return p.ChatWithTools(ctx, messages, model, systemPrompt, nil, opts, callback)
}

func (p *OpenAIProvider) ChatWithTools(ctx context.Context, messages []models.Message, model string, systemPrompt string, tools []Tool, opts *ChatOptions, callback StreamCallback) error {
	startTime := time.Now()
	var ttfb float64
	var outputTokens int

	// Check if this is a reasoning model (o1, o3, o4 series)
	isReasoning := isReasoningModel(model)

	// Convert messages to OpenAI format
	openaiMsgs := make([]openaiMessage, 0, len(messages)+1)

	// Add system/developer message
	if systemPrompt != "" {
		role := "system"
		if isReasoning {
			role = "developer" // o-series models use "developer" instead of "system"
		}
		openaiMsgs = append(openaiMsgs, openaiMessage{
			Role:    role,
			Content: systemPrompt,
		})
	}

	for _, msg := range messages {
		if msg.Role == "system" {
			continue
		}

		// Handle tool results - OpenAI expects separate "tool" role messages
		if len(msg.ToolResults) > 0 {
			for _, tr := range msg.ToolResults {
				openaiMsgs = append(openaiMsgs, openaiMessage{
					Role:       "tool",
					Content:    tr.Content,
					ToolCallID: tr.ToolUseID,
				})
			}
			continue
		}

		// Check if we need multimodal content
		hasImages := false
		for _, att := range msg.Attachments {
			if strings.HasPrefix(att.MimeType, "image/") {
				hasImages = true
				break
			}
		}

		// Build the message
		var openaiMsg openaiMessage
		openaiMsg.Role = msg.Role

		if hasImages {
			// Multimodal message
			parts := []openaiContentPart{}

			if msg.Content != "" {
				parts = append(parts, openaiContentPart{
					Type: "text",
					Text: msg.Content,
				})
			}

			for _, att := range msg.Attachments {
				if strings.HasPrefix(att.MimeType, "image/") && att.Data != "" {
					parts = append(parts, openaiContentPart{
						Type: "image_url",
						ImageURL: &openaiImageURL{
							URL:    fmt.Sprintf("data:%s;base64,%s", att.MimeType, att.Data),
							Detail: "auto",
						},
					})
				}
			}
			openaiMsg.Content = parts
		} else {
			// Text only (can be empty string for assistant messages with only tool calls)
			openaiMsg.Content = msg.Content
		}

		// Handle tool calls in assistant messages
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			openaiMsg.ToolCalls = make([]openaiMessageToolCall, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				// Serialize arguments to JSON string
				argsJSON, err := json.Marshal(tc.Arguments)
				if err != nil {
					argsJSON = []byte("{}")
				}
				openaiMsg.ToolCalls[i] = openaiMessageToolCall{
					ID:   tc.ID,
					Type: "function",
				}
				openaiMsg.ToolCalls[i].Function.Name = tc.Name
				openaiMsg.ToolCalls[i].Function.Arguments = string(argsJSON)
			}
		}

		openaiMsgs = append(openaiMsgs, openaiMsg)
	}

	// Build request
	req := openaiRequest{
		Model:    model,
		Messages: openaiMsgs,
		Stream:   true,
		StreamOptions: &openaiStreamOptions{
			IncludeUsage: true,
		},
	}

	// Handle token limits - OpenAI now uses max_completion_tokens for all models
	req.MaxCompletionTokens = 4096
	if opts != nil && opts.MaxTokens != nil {
		req.MaxCompletionTokens = *opts.MaxTokens
	}

	// Handle reasoning model specific parameters
	if isReasoning {
		// o-series models need higher default and reasoning_effort
		if req.MaxCompletionTokens < 16000 {
			req.MaxCompletionTokens = 16000
		}

		// Set reasoning effort based on thinking budget
		req.ReasoningEffort = "medium" // default
		if opts != nil && opts.ThinkingBudget != "" {
			switch opts.ThinkingBudget {
			case "low":
				req.ReasoningEffort = "low"
			case "medium":
				req.ReasoningEffort = "medium"
			case "high":
				req.ReasoningEffort = "high"
			}
		} else if opts != nil && opts.EnableThinking {
			req.ReasoningEffort = "high"
		}
		// Note: Don't set temperature for o-series models
	} else {
		// Set temperature for non-reasoning models
		if opts != nil && opts.Temperature != nil {
			req.Temperature = opts.Temperature
		}
	}

	// Add tools if provided
	if len(tools) > 0 {
		req.Tools = make([]openaiTool, len(tools))
		for i, t := range tools {
			// Normalize InputSchema for OpenAI - ensure object type has properties
			params := normalizeToolSchema(t.InputSchema)
			req.Tools[i] = openaiTool{
				Type: "function",
				Function: openaiToolFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  params,
				},
			}
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Send debug event
	callback(models.StreamEvent{
		Type: "debug",
		Data: map[string]interface{}{
			"request": map[string]interface{}{
				"url":    p.baseURL,
				"method": "POST",
				"body":   req,
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
	firstChunk := true

	// Track accumulated tool calls (OpenAI sends them in pieces)
	toolCalls := make(map[int]*struct {
		ID        string
		Name      string
		Arguments strings.Builder
	})

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var streamResp openaiStreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			log.Printf("OpenAI: Failed to parse SSE data: %v, data: %s", err, truncateForLog(data, 200))
			continue
		}

		if len(streamResp.Choices) > 0 {
			delta := streamResp.Choices[0].Delta

			// Handle regular content
			if delta.Content != "" {
				if firstChunk {
					ttfb = float64(time.Since(startTime).Milliseconds())
					firstChunk = false
				}

				outputTokens += len(strings.Fields(delta.Content))
				callback(models.StreamEvent{
					Type:    "delta",
					Content: delta.Content,
				})
			}

			// Handle tool calls (streaming)
			for _, tc := range delta.ToolCalls {
				if firstChunk {
					ttfb = float64(time.Since(startTime).Milliseconds())
					firstChunk = false
				}

				// Initialize or get existing tool call
				if _, exists := toolCalls[tc.Index]; !exists {
					toolCalls[tc.Index] = &struct {
						ID        string
						Name      string
						Arguments strings.Builder
					}{}
				}

				call := toolCalls[tc.Index]

				// First chunk has ID and name
				if tc.ID != "" {
					call.ID = tc.ID
				} else if call.ID == "" {
					// Fallback: generate unique ID if server doesn't provide one
					call.ID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), tc.Index)
				}
				if tc.Function.Name != "" {
					call.Name = tc.Function.Name
					// Emit tool_start when we get the name
					callback(models.StreamEvent{
						Type: "tool_start",
						Data: map[string]interface{}{
							"id":   call.ID,
							"name": call.Name,
						},
					})
				}

				// Accumulate arguments
				if tc.Function.Arguments != "" {
					call.Arguments.WriteString(tc.Function.Arguments)
					callback(models.StreamEvent{
						Type: "tool_delta",
						Data: map[string]interface{}{
							"partial_json": tc.Function.Arguments,
						},
					})
				}
			}

			// Log finish reason for debugging
			finishReason := streamResp.Choices[0].FinishReason
			if finishReason != "" {
				log.Printf("OpenAI: Stream finished with reason: %s, tool_calls: %d", finishReason, len(toolCalls))
			}

			// Check finish reason for tool_calls
			if finishReason == "tool_calls" {
				// Emit tool_complete events with parsed arguments
				for _, call := range toolCalls {
					var args map[string]interface{}
					if err := json.Unmarshal([]byte(call.Arguments.String()), &args); err != nil {
						log.Printf("Failed to parse tool arguments: %v", err)
						args = nil
					}
					log.Printf("OpenAI: Emitting tool_complete for %s (id=%s)", call.Name, call.ID)
					callback(models.StreamEvent{
						Type: "tool_complete",
						Data: map[string]interface{}{
							"id":        call.ID,
							"name":      call.Name,
							"arguments": args,
						},
					})
				}
			}
		}

		// Check for usage info (sent at end with stream_options)
		if streamResp.Usage != nil {
			inputTokens = streamResp.Usage.PromptTokens
			outputTokens = streamResp.Usage.CompletionTokens
		}
	}

// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("OpenAI: Scanner error: %v", err)
	}

	// Log warning if no content was received
	if firstChunk {
		log.Printf("OpenAI: Warning - stream ended without any content or tool calls (input_tokens=%d, output_tokens=%d)", inputTokens, outputTokens)
	}

	totalLatency := float64(time.Since(startTime).Milliseconds())
	tokensPerSec := 0.0
	if totalLatency > ttfb && outputTokens > 0 {
		tokensPerSec = float64(outputTokens) / ((totalLatency - ttfb) / 1000)
	}

	callback(models.StreamEvent{
		Type: "metrics",
		Metrics: &models.Metrics{
			InputTokens:     inputTokens,
			OutputTokens:    outputTokens,
			TotalTokens:     inputTokens + outputTokens,
			TimeToFirstByte: ttfb,
			TotalLatency:    totalLatency,
			TokensPerSecond: tokensPerSec,
		},
	})

	callback(models.StreamEvent{Type: "done"})

	return nil
}

// normalizeToolSchema ensures the schema is valid for OpenAI API
// OpenAI requires object type schemas to have a "properties" field
// and array type schemas to have an "items" field
func normalizeToolSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		// Empty schema - return minimal valid object schema
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	// Make a copy to avoid modifying original
	result := make(map[string]interface{})
	for k, v := range schema {
		result[k] = v
	}

	// Check if type is object (or defaults to object if not specified)
	schemaType, hasType := result["type"].(string)
	if !hasType {
		schemaType = "object"
		result["type"] = "object"
	}

	// If type is object, ensure properties exists and normalize nested schemas
	if schemaType == "object" {
		if props, hasProps := result["properties"].(map[string]interface{}); hasProps {
			// Recursively normalize nested property schemas
			normalizedProps := make(map[string]interface{})
			for propName, propSchema := range props {
				if propSchemaMap, ok := propSchema.(map[string]interface{}); ok {
					normalizedProps[propName] = normalizeToolSchema(propSchemaMap)
				} else {
					normalizedProps[propName] = propSchema
				}
			}
			result["properties"] = normalizedProps
		} else {
			result["properties"] = map[string]interface{}{}
		}
	}

	// If type is array, ensure items exists
	if schemaType == "array" {
		if _, hasItems := result["items"]; !hasItems {
			// Default to string items if not specified
			result["items"] = map[string]interface{}{
				"type": "string",
			}
		} else if itemsMap, ok := result["items"].(map[string]interface{}); ok {
			// Recursively normalize items schema
			result["items"] = normalizeToolSchema(itemsMap)
		}
	}

	return result
}

func (p *OpenAIProvider) CountTokens(messages []models.Message) (int, error) {
	// Rough estimation: ~4 chars per token for English
	total := 0
	for _, msg := range messages {
		total += len(msg.Content) / 4
	}
	return total, nil
}

// truncateForLog truncates a string for logging purposes
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
