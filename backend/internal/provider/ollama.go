package provider

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

	"github.com/spetr/chatapp/internal/models"
)

// Known thinking models - value indicates if model uses budget levels (low/medium/high) vs boolean
var thinkingModels = map[string]bool{
	"deepseek-r1": false, // uses boolean
	"qwen3":       false, // uses boolean
	"qwq":         false, // uses boolean
	"marco-o1":    false, // uses boolean
	"gpt-oss":     true,  // uses low/medium/high budget levels
}

type OllamaProvider struct {
	baseURL string
	models  []string
	client  *http.Client
}

func NewOllamaProvider(modelList []string, baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &OllamaProvider{
		baseURL: baseURL,
		models:  modelList,
		client: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) Models() []string {
	return p.models
}

// Native Ollama API types
type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Images    []string         `json:"images,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"` // For assistant messages with tool calls
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Think    interface{}     `json:"think,omitempty"` // bool for most models, string (low/medium/high) for gpt-oss
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Options  *ollamaOptions  `json:"options,omitempty"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type ollamaOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	TopK        *int     `json:"top_k,omitempty"`
	NumPredict  *int     `json:"num_predict,omitempty"` // max tokens
	Seed        *int     `json:"seed,omitempty"`
}

type ollamaToolCall struct {
	Function struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	} `json:"function"`
}

type ollamaStreamResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Message   struct {
		Role      string           `json:"role"`
		Content   string           `json:"content"`
		Thinking  string           `json:"thinking,omitempty"`
		ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	} `json:"message"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
}

// Check if model supports thinking
func supportsThinking(model string) bool {
	modelLower := strings.ToLower(model)
	for prefix := range thinkingModels {
		if strings.HasPrefix(modelLower, prefix) {
			return true
		}
	}
	return false
}

// Check if model uses budget levels (low/medium/high) instead of boolean
func usesBudgetLevels(model string) bool {
	modelLower := strings.ToLower(model)
	for prefix, usesBudget := range thinkingModels {
		if strings.HasPrefix(modelLower, prefix) {
			return usesBudget
		}
	}
	return false
}

// Get thinking value for request - returns appropriate type based on model
func getThinkingValue(model string, enableThinking bool, thinkingBudget string) interface{} {
	if !enableThinking {
		return nil // Don't include think parameter
	}

	if usesBudgetLevels(model) {
		// GPT-OSS style models use low/medium/high
		if thinkingBudget == "" {
			return "medium" // default
		}
		return thinkingBudget
	}

	// Standard models use boolean
	return true
}

func (p *OllamaProvider) Chat(ctx context.Context, messages []models.Message, model string, systemPrompt string, opts *ChatOptions, callback StreamCallback) error {
	return p.ChatWithTools(ctx, messages, model, systemPrompt, nil, opts, callback)
}

func (p *OllamaProvider) ChatWithTools(ctx context.Context, messages []models.Message, model string, systemPrompt string, tools []Tool, opts *ChatOptions, callback StreamCallback) error {
	startTime := time.Now()
	var ttfb float64
	var inputTokens, outputTokens int
	firstChunk := true

	// Determine thinking settings
	enableThinking := false
	thinkingBudget := ""
	if opts != nil && opts.EnableThinking && supportsThinking(model) {
		enableThinking = true
		thinkingBudget = opts.ThinkingBudget
	}

	// Get the appropriate thinking value (bool or string based on model)
	thinkValue := getThinkingValue(model, enableThinking, thinkingBudget)

	// Convert messages to Ollama native format
	ollamaMsgs := make([]ollamaMessage, 0, len(messages)+1)

	// Add system message if present
	if systemPrompt != "" {
		ollamaMsgs = append(ollamaMsgs, ollamaMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// Convert messages
	for _, msg := range messages {
		// Handle tool results - Ollama expects separate "tool" role messages
		if len(msg.ToolResults) > 0 {
			for _, tr := range msg.ToolResults {
				ollamaMsgs = append(ollamaMsgs, ollamaMessage{
					Role:    "tool",
					Content: tr.Content,
				})
			}
			continue
		}

		ollamaMsg := ollamaMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}

		// Handle tool calls in assistant messages
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			ollamaMsg.ToolCalls = make([]ollamaToolCall, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				ollamaMsg.ToolCalls[i] = ollamaToolCall{
					Function: struct {
						Name      string                 `json:"name"`
						Arguments map[string]interface{} `json:"arguments"`
					}{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				}
			}
		}

		// Handle image attachments
		for _, att := range msg.Attachments {
			if strings.HasPrefix(att.MimeType, "image/") && att.Data != "" {
				ollamaMsg.Images = append(ollamaMsg.Images, att.Data)
			}
		}

		ollamaMsgs = append(ollamaMsgs, ollamaMsg)
	}

	// Build request using native Ollama API
	ollamaReq := ollamaRequest{
		Model:    model,
		Messages: ollamaMsgs,
		Stream:   true,
		Think:    thinkValue,
	}

	// Add options from ChatOptions
	if opts != nil {
		ollamaOpts := &ollamaOptions{}
		hasOptions := false

		if opts.Temperature != nil {
			ollamaOpts.Temperature = opts.Temperature
			hasOptions = true
		}
		if opts.TopP != nil {
			ollamaOpts.TopP = opts.TopP
			hasOptions = true
		}
		if opts.TopK != nil {
			ollamaOpts.TopK = opts.TopK
			hasOptions = true
		}
		if opts.MaxTokens != nil {
			ollamaOpts.NumPredict = opts.MaxTokens
			hasOptions = true
		}
		if opts.Seed != nil {
			ollamaOpts.Seed = opts.Seed
			hasOptions = true
		}

		if hasOptions {
			ollamaReq.Options = ollamaOpts
		}
	}

	// Add tools if provided
	if len(tools) > 0 {
		ollamaReq.Tools = make([]ollamaTool, len(tools))
		for i, t := range tools {
			// Normalize schema - ensure object type has properties
			params := t.InputSchema
			if params == nil {
				params = map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				}
			} else {
				// Check if type is object and properties is missing
				if schemaType, ok := params["type"].(string); ok && schemaType == "object" {
					if _, hasProps := params["properties"]; !hasProps {
						// Make a copy and add empty properties
						paramsCopy := make(map[string]interface{})
						for k, v := range params {
							paramsCopy[k] = v
						}
						paramsCopy["properties"] = map[string]interface{}{}
						params = paramsCopy
					}
				}
			}

			ollamaReq.Tools[i] = ollamaTool{
				Type: "function",
				Function: ollamaToolFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  params,
				},
			}
		}
	}

	// Send debug event
	callback(models.StreamEvent{
		Type: "debug",
		Data: map[string]interface{}{
			"request": map[string]interface{}{
				"url":              p.baseURL + "/api/chat",
				"method":           "POST",
				"body":             ollamaReq,
				"thinking_enabled": enableThinking,
				"thinking_budget":  thinkingBudget,
				"tools":            ollamaReq.Tools,
			},
		},
	})

	callback(models.StreamEvent{Type: "start"})

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		callback(models.StreamEvent{
			Type:  "error",
			Error: fmt.Sprintf("request failed: %v", err),
		})
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("Ollama API error: %s - %s", resp.Status, string(body))
		callback(models.StreamEvent{
			Type:  "error",
			Error: errMsg,
		})
		return fmt.Errorf("%s", errMsg)
	}

	// Read NDJSON stream (native Ollama format)
	reader := bufio.NewReader(resp.Body)
	var lastThinking string

	for {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read error: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var streamResp ollamaStreamResponse
		if err := json.Unmarshal([]byte(line), &streamResp); err != nil {
			continue
		}

		// Handle thinking content (streamed separately)
		if streamResp.Message.Thinking != "" && streamResp.Message.Thinking != lastThinking {
			var thinkingToEmit string

			// Check if new thinking extends the previous (accumulated mode)
			if len(streamResp.Message.Thinking) > len(lastThinking) &&
				strings.HasPrefix(streamResp.Message.Thinking, lastThinking) {
				// Extract only the new delta
				thinkingToEmit = streamResp.Message.Thinking[len(lastThinking):]
			} else {
				// Different thinking content - emit as-is (delta mode or replacement)
				// For delta mode, we emit the whole new content
				thinkingToEmit = streamResp.Message.Thinking
			}

			if thinkingToEmit != "" {
				if firstChunk {
					ttfb = float64(time.Since(startTime).Milliseconds())
					firstChunk = false
				}
				callback(models.StreamEvent{
					Type:    "thinking",
					Content: thinkingToEmit,
				})
			}
			lastThinking = streamResp.Message.Thinking
		}

		// Handle regular content
		if streamResp.Message.Content != "" {
			if firstChunk {
				ttfb = float64(time.Since(startTime).Milliseconds())
				firstChunk = false
			}

			callback(models.StreamEvent{
				Type:    "delta",
				Content: streamResp.Message.Content,
			})
		}

		// Handle tool calls - Ollama sends complete tool calls (not streamed pieces)
		if len(streamResp.Message.ToolCalls) > 0 {
			for i, tc := range streamResp.Message.ToolCalls {
				// Generate unique ID using timestamp + index to avoid collisions across iterations
				toolID := fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), i)

				// Emit tool_start event
				callback(models.StreamEvent{
					Type: "tool_start",
					Data: map[string]interface{}{
						"id":        toolID,
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					},
				})

				// Emit tool_complete immediately since Ollama sends complete tool calls
				callback(models.StreamEvent{
					Type: "tool_complete",
					Data: map[string]interface{}{
						"id":        toolID,
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					},
				})
			}
		}

		// Check if done
		if streamResp.Done {
			inputTokens = streamResp.PromptEvalCount
			outputTokens = streamResp.EvalCount
			break
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

func (p *OllamaProvider) CountTokens(messages []models.Message) (int, error) {
	// Rough estimation: ~4 chars per token
	total := 0
	for _, msg := range messages {
		total += len(msg.Content) / 4
	}
	return total, nil
}
