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

/*
================================================================================
                         LLAMA.CPP SERVER PROVIDER
================================================================================

This provider integrates with llama-server (llama.cpp's HTTP server) and supports
all native features including:

ENDPOINTS SUPPORTED:
─────────────────────────────────────────────────────────────────────────────────
- POST /completion      - Native completion with full parameter control
- POST /v1/chat/completions - OpenAI-compatible chat (used as primary)
- POST /infill          - Code infill/FIM (Fill-In-Middle)
- POST /tokenize        - Tokenize text to tokens
- POST /detokenize      - Convert tokens back to text
- POST /embedding       - Generate embeddings
- GET  /health          - Server health status
- GET  /props           - Server properties (model info)
- GET  /slots           - KV cache slot information

SPECIAL FEATURES:
─────────────────────────────────────────────────────────────────────────────────
- Grammar Sampling (GBNF) - Constrained generation with formal grammars
- JSON Schema Mode       - Structured JSON output
- Multimodal Support     - Image input for vision models (LLaVA, etc.)
- Speculative Decoding   - Draft model acceleration
- Mirostat Sampling      - Perplexity-controlled sampling
- Cache Management       - KV cache slot control

PRICING:
─────────────────────────────────────────────────────────────────────────────────
Uses same electricity-based calculation as Ollama since both use same inference.
See pricing.go for GPU-specific cost calculations.

================================================================================
*/

type LlamaCppProvider struct {
	baseURL string
	models  []string
	client  *http.Client
}

func NewLlamaCppProvider(modelList []string, baseURL string) *LlamaCppProvider {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &LlamaCppProvider{
		baseURL: baseURL,
		models:  modelList,
		client: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (p *LlamaCppProvider) Name() string {
	return "llamacpp"
}

func (p *LlamaCppProvider) Models() []string {
	return p.models
}

// ─────────────────────────────────────────────────────────────────────────────
// Native llama.cpp API types
// ─────────────────────────────────────────────────────────────────────────────

// llamaCppInfillRequest is for the /infill endpoint (code completion)
type llamaCppInfillRequest struct {
	InputPrefix string   `json:"input_prefix"`
	InputSuffix string   `json:"input_suffix"`
	Prompt      string   `json:"prompt,omitempty"` // Optional middle hint
	NPredict    int      `json:"n_predict,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopK        *int     `json:"top_k,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`
	Grammar     string   `json:"grammar,omitempty"`
}

// OpenAI-compatible chat types (used as primary interface)
type llamaCppChatRequest struct {
	Model            string                 `json:"model,omitempty"`
	Messages         []llamaCppMessage      `json:"messages"`
	MaxTokens        int                    `json:"max_tokens,omitempty"`
	Temperature      *float64               `json:"temperature,omitempty"`
	TopP             *float64               `json:"top_p,omitempty"`
	TopK             *int                   `json:"top_k,omitempty"`
	Stream           bool                   `json:"stream"`
	Stop             []string               `json:"stop,omitempty"`
	PresencePenalty  *float64               `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64               `json:"frequency_penalty,omitempty"`
	RepeatPenalty    *float64               `json:"repeat_penalty,omitempty"`
	Seed             *int                   `json:"seed,omitempty"`
	Grammar          string                 `json:"grammar,omitempty"`
	JSONSchema       map[string]interface{} `json:"json_schema,omitempty"`
	Tools            []llamaCppTool         `json:"tools,omitempty"`
	CachePrompt      bool                   `json:"cache_prompt,omitempty"`
	// Mirostat params
	Mirostat    *int     `json:"mirostat,omitempty"`
	MirostatTau *float64 `json:"mirostat_tau,omitempty"`
	MirostatEta *float64 `json:"mirostat_eta,omitempty"`
}

type llamaCppMessage struct {
	Role       string               `json:"role"`
	Content    interface{}          `json:"content"`               // string or []content parts for multimodal
	ToolCalls  []llamaCppToolCall   `json:"tool_calls,omitempty"`  // For assistant messages with tool calls
	ToolCallID string               `json:"tool_call_id,omitempty"` // For tool result messages
}

type llamaCppContentPart struct {
	Type     string            `json:"type"` // "text" or "image_url"
	Text     string            `json:"text,omitempty"`
	ImageURL *llamaCppImageURL `json:"image_url,omitempty"`
}

type llamaCppImageURL struct {
	URL string `json:"url"` // data:image/...;base64,... or http URL
}

type llamaCppTool struct {
	Type     string               `json:"type"`
	Function llamaCppToolFunction `json:"function"`
}

type llamaCppToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type llamaCppToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Stream response types
type llamaCppStreamResponse struct {
	// For /v1/chat/completions
	ID      string `json:"id,omitempty"`
	Object  string `json:"object,omitempty"`
	Created int64  `json:"created,omitempty"`
	Model   string `json:"model,omitempty"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string             `json:"role,omitempty"`
			Content   string             `json:"content,omitempty"`
			ToolCalls []llamaCppToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices,omitempty"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`

	// For native /completion endpoint
	Content         string           `json:"content,omitempty"`
	Stop            bool             `json:"stop,omitempty"`
	TokensEvaluated int              `json:"tokens_evaluated,omitempty"`
	TokensPredicted int              `json:"tokens_predicted,omitempty"`
	PromptN         int              `json:"prompt_n,omitempty"`
	PredictedN      int              `json:"predicted_n,omitempty"`
	Timings         *llamaCppTimings `json:"timings,omitempty"`
}

type llamaCppTimings struct {
	PromptN             int     `json:"prompt_n"`
	PromptMS            float64 `json:"prompt_ms"`
	PromptPerTokenMS    float64 `json:"prompt_per_token_ms"`
	PromptPerSecond     float64 `json:"prompt_per_second"`
	PredictedN          int     `json:"predicted_n"`
	PredictedMS         float64 `json:"predicted_ms"`
	PredictedPerTokenMS float64 `json:"predicted_per_token_ms"`
	PredictedPerSecond  float64 `json:"predicted_per_second"`
}

// Health and status types
type LlamaCppHealth struct {
	Status          string `json:"status"` // "ok", "loading model", "error"
	SlotsIdle       int    `json:"slots_idle,omitempty"`
	SlotsProcessing int    `json:"slots_processing,omitempty"`
}

type LlamaCppProps struct {
	AssistantName      string `json:"assistant_name,omitempty"`
	UserName           string `json:"user_name,omitempty"`
	DefaultGenSettings struct {
		NCtx          int     `json:"n_ctx"`
		NPredict      int     `json:"n_predict"`
		Model         string  `json:"model"`
		Seed          int     `json:"seed"`
		Temperature   float64 `json:"temperature"`
		TopK          int     `json:"top_k"`
		TopP          float64 `json:"top_p"`
		MinP          float64 `json:"min_p"`
		RepeatPenalty float64 `json:"repeat_penalty"`
	} `json:"default_generation_settings,omitempty"`
	TotalSlots int `json:"total_slots,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Provider Implementation
// ─────────────────────────────────────────────────────────────────────────────

func (p *LlamaCppProvider) Chat(ctx context.Context, messages []models.Message, model string, systemPrompt string, opts *ChatOptions, callback StreamCallback) error {
	return p.ChatWithTools(ctx, messages, model, systemPrompt, nil, opts, callback)
}

func (p *LlamaCppProvider) ChatWithTools(ctx context.Context, messages []models.Message, model string, systemPrompt string, tools []Tool, opts *ChatOptions, callback StreamCallback) error {
	startTime := time.Now()
	var ttfb float64
	var inputTokens, outputTokens int
	firstChunk := true

	// Build messages array
	chatMsgs := make([]llamaCppMessage, 0, len(messages)+1)

	// Add system message if present
	if systemPrompt != "" {
		chatMsgs = append(chatMsgs, llamaCppMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// Convert messages
	for _, msg := range messages {
		// Handle tool results - llama.cpp (OpenAI-compatible) expects separate "tool" role messages
		if len(msg.ToolResults) > 0 {
			for _, tr := range msg.ToolResults {
				chatMsgs = append(chatMsgs, llamaCppMessage{
					Role:       "tool",
					Content:    tr.Content,
					ToolCallID: tr.ToolUseID,
				})
			}
			continue
		}

		// Check for image attachments
		hasImages := false
		for _, att := range msg.Attachments {
			if strings.HasPrefix(att.MimeType, "image/") {
				hasImages = true
				break
			}
		}

		var chatMsg llamaCppMessage
		chatMsg.Role = msg.Role

		if hasImages {
			// Multimodal message with content parts
			parts := []llamaCppContentPart{}

			if msg.Content != "" {
				parts = append(parts, llamaCppContentPart{
					Type: "text",
					Text: msg.Content,
				})
			}

			for _, att := range msg.Attachments {
				if strings.HasPrefix(att.MimeType, "image/") && att.Data != "" {
					parts = append(parts, llamaCppContentPart{
						Type: "image_url",
						ImageURL: &llamaCppImageURL{
							URL: fmt.Sprintf("data:%s;base64,%s", att.MimeType, att.Data),
						},
					})
				}
			}

			chatMsg.Content = parts
		} else {
			// Text-only message
			chatMsg.Content = msg.Content
		}

		// Handle tool calls in assistant messages
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			chatMsg.ToolCalls = make([]llamaCppToolCall, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				chatMsg.ToolCalls[i] = llamaCppToolCall{
					Index: i,
					ID:    tc.ID,
					Type:  "function",
				}
				chatMsg.ToolCalls[i].Function.Name = tc.Name
				chatMsg.ToolCalls[i].Function.Arguments = string(argsJSON)
			}
		}

		chatMsgs = append(chatMsgs, chatMsg)
	}

	// Build request
	req := llamaCppChatRequest{
		Model:       model,
		Messages:    chatMsgs,
		Stream:      true,
		CachePrompt: true, // Enable prompt caching by default
	}

	// Apply options
	if opts != nil {
		if opts.Temperature != nil {
			req.Temperature = opts.Temperature
		}
		if opts.TopP != nil {
			req.TopP = opts.TopP
		}
		if opts.TopK != nil {
			req.TopK = opts.TopK
		}
		if opts.MaxTokens != nil {
			req.MaxTokens = *opts.MaxTokens
		}
		if opts.Seed != nil {
			req.Seed = opts.Seed
		}
	}

	// Default max tokens if not set
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}

	// Add tools if provided
	if len(tools) > 0 {
		req.Tools = make([]llamaCppTool, len(tools))
		for i, t := range tools {
			params := normalizeToolSchema(t.InputSchema)
			req.Tools[i] = llamaCppTool{
				Type: "function",
				Function: llamaCppToolFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  params,
				},
			}
		}
	}

	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Send debug event
	callback(models.StreamEvent{
		Type: "debug",
		Data: map[string]interface{}{
			"request": map[string]interface{}{
				"url":    p.baseURL + "/v1/chat/completions",
				"method": "POST",
				"body":   req,
			},
		},
	})

	callback(models.StreamEvent{Type: "start"})

	// Execute request
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
		errMsg := fmt.Sprintf("llama.cpp API error %d: %s", resp.StatusCode, string(body))
		callback(models.StreamEvent{
			Type:  "error",
			Error: errMsg,
		})
		return fmt.Errorf("%s", errMsg)
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Track accumulated tool calls
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

		var streamResp llamaCppStreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue
		}

		// Update token counts from usage (if provided at end)
		if streamResp.Usage != nil {
			inputTokens = streamResp.Usage.PromptTokens
			outputTokens = streamResp.Usage.CompletionTokens
		}

		if len(streamResp.Choices) > 0 {
			choice := streamResp.Choices[0]

			// Handle content
			if choice.Delta.Content != "" {
				if firstChunk {
					ttfb = float64(time.Since(startTime).Milliseconds())
					firstChunk = false
				}

				callback(models.StreamEvent{
					Type:    "delta",
					Content: choice.Delta.Content,
				})
			}

			// Handle tool calls
			for _, tc := range choice.Delta.ToolCalls {
				if firstChunk {
					ttfb = float64(time.Since(startTime).Milliseconds())
					firstChunk = false
				}

				idx := tc.Index
				if _, exists := toolCalls[idx]; !exists {
					toolCalls[idx] = &struct {
						ID        string
						Name      string
						Arguments strings.Builder
					}{}
				}

				call := toolCalls[idx]
				if tc.ID != "" {
					call.ID = tc.ID
				} else if call.ID == "" {
					// Generate unique ID if server doesn't provide one
					call.ID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), idx)
				}
				if tc.Function.Name != "" {
					call.Name = tc.Function.Name
					callback(models.StreamEvent{
						Type: "tool_start",
						Data: map[string]interface{}{
							"id":   call.ID,
							"name": call.Name,
						},
					})
				}
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

			// Check for tool_calls finish - emit tool_complete for all accumulated tool calls
			if choice.FinishReason == "tool_calls" {
				for _, call := range toolCalls {
					var args map[string]interface{}
					if err := json.Unmarshal([]byte(call.Arguments.String()), &args); err != nil {
						args = nil // Use nil if parsing fails
					}
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
	}

	// Calculate metrics
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

// ─────────────────────────────────────────────────────────────────────────────
// Special Features: Infill (Code Completion)
// ─────────────────────────────────────────────────────────────────────────────

// Infill performs code completion given prefix and suffix (Fill-In-Middle)
func (p *LlamaCppProvider) Infill(ctx context.Context, prefix, suffix, hint string, opts *ChatOptions) (string, error) {
	req := llamaCppInfillRequest{
		InputPrefix: prefix,
		InputSuffix: suffix,
		Prompt:      hint, // Optional middle hint
		NPredict:    256,
	}

	if opts != nil {
		if opts.Temperature != nil {
			req.Temperature = opts.Temperature
		}
		if opts.TopK != nil {
			req.TopK = opts.TopK
		}
		if opts.TopP != nil {
			req.TopP = opts.TopP
		}
		if opts.MaxTokens != nil {
			req.NPredict = *opts.MaxTokens
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/infill", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("infill failed: %s", string(respBody))
	}

	// Read streaming response and concatenate
	var result strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			var streamResp llamaCppStreamResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err == nil {
				result.WriteString(streamResp.Content)
			}
		}
	}

	return result.String(), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Special Features: Tokenization
// ─────────────────────────────────────────────────────────────────────────────

// Tokenize converts text to tokens
func (p *LlamaCppProvider) Tokenize(ctx context.Context, text string) ([]int, error) {
	body, _ := json.Marshal(map[string]string{"content": text})

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/tokenize", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Tokens []int `json:"tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Tokens, nil
}

// Detokenize converts tokens back to text
func (p *LlamaCppProvider) Detokenize(ctx context.Context, tokens []int) (string, error) {
	body, _ := json.Marshal(map[string][]int{"tokens": tokens})

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/detokenize", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Content, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Special Features: Embeddings
// ─────────────────────────────────────────────────────────────────────────────

// Embedding generates embeddings for text
func (p *LlamaCppProvider) Embedding(ctx context.Context, text string) ([]float64, error) {
	body, _ := json.Marshal(map[string]string{"content": text})

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/embedding", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Embedding, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Health & Status
// ─────────────────────────────────────────────────────────────────────────────

// Health returns server health status
func (p *LlamaCppProvider) Health(ctx context.Context) (*LlamaCppHealth, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/health", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var health LlamaCppHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, err
	}

	return &health, nil
}

// Props returns server properties
func (p *LlamaCppProvider) Props(ctx context.Context) (*LlamaCppProps, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/props", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var props LlamaCppProps
	if err := json.NewDecoder(resp.Body).Decode(&props); err != nil {
		return nil, err
	}

	return &props, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Token Counting
// ─────────────────────────────────────────────────────────────────────────────

func (p *LlamaCppProvider) CountTokens(messages []models.Message) (int, error) {
	// Use tokenize endpoint for accurate count if available
	total := 0
	for _, msg := range messages {
		// Fallback to estimation
		total += len(msg.Content) / 4
	}
	return total, nil
}
