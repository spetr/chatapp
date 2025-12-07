package models

import (
	"time"
)

type Conversation struct {
	ID           string                `json:"id"`
	Title        string                `json:"title"`
	Provider     string                `json:"provider"`
	Model        string                `json:"model"`
	SystemPrompt string                `json:"system_prompt"`
	Settings     *ConversationSettings `json:"settings,omitempty"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

// ConversationSettings contains all configurable parameters for a conversation
type ConversationSettings struct {
	// Generation parameters
	Temperature      *float64 `json:"temperature,omitempty"`       // 0.0-2.0, default varies by provider
	MaxTokens        *int     `json:"max_tokens,omitempty"`        // Max response length
	TopP             *float64 `json:"top_p,omitempty"`             // 0.0-1.0, nucleus sampling
	TopK             *int     `json:"top_k,omitempty"`             // For Ollama
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"` // -2.0 to 2.0
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`  // -2.0 to 2.0
	StopSequences    []string `json:"stop_sequences,omitempty"`    // Stop generation at these

	// Feature toggles
	Stream          *bool `json:"stream,omitempty"`           // Enable streaming (default true)
	EnableThinking  *bool `json:"enable_thinking,omitempty"`  // Enable reasoning/thinking mode
	EnableTools     *bool `json:"enable_tools,omitempty"`     // Enable tool/function calling
	EnableCitations *bool `json:"enable_citations,omitempty"` // Enable document citations (Claude)

	// Context management
	ContextLength    *int `json:"context_length,omitempty"`     // Custom context window
	MaxHistoryLength *int `json:"max_history_length,omitempty"` // Max messages to include

	// Response format
	ResponseFormat *string `json:"response_format,omitempty"` // "text" or "json_object"

	// Thinking budget - "low", "medium", "high" for Ollama, or numeric string for Claude
	ThinkingBudget *string `json:"thinking_budget,omitempty"`

	// Ollama/llama.cpp specific
	NumCtx        *int     `json:"num_ctx,omitempty"`        // Context window size
	NumPredict    *int     `json:"num_predict,omitempty"`    // Max tokens to predict
	RepeatPenalty *float64 `json:"repeat_penalty,omitempty"` // Repetition penalty
	Seed          *int     `json:"seed,omitempty"`           // Random seed for reproducibility
	Grammar       *string  `json:"grammar,omitempty"`        // GBNF grammar for structured output
}

type Message struct {
	ID             string       `json:"id"`
	ConversationID string       `json:"conversation_id"`
	Role           string       `json:"role"` // user, assistant, system
	Content        string       `json:"content"`
	Attachments    []Attachment `json:"attachments,omitempty"`
	Metrics        *Metrics     `json:"metrics,omitempty"`
	ParentID       *string      `json:"parent_id,omitempty"` // For forking
	CreatedAt      time.Time    `json:"created_at"`
	// Tool call fields (not persisted, used during streaming)
	ToolCalls   []ToolCallInfo   `json:"tool_calls,omitempty"`
	ToolResults []ToolResultInfo `json:"tool_results,omitempty"`
}

// ToolCallInfo represents a tool call made by the assistant
type ToolCallInfo struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
	Result    string                 `json:"result,omitempty"`    // Tool execution result
	IsError   bool                   `json:"is_error,omitempty"`  // Whether the tool call resulted in an error
}

// ToolResultInfo represents the result of a tool call
type ToolResultInfo struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

type Attachment struct {
	ID        string `json:"id"`
	MessageID string `json:"message_id"`
	Filename  string `json:"filename"`
	MimeType  string `json:"mime_type"`
	Size      int64  `json:"size"`
	Path      string `json:"path"`
	// For images, can include base64 data
	Data string `json:"data,omitempty"`
}

// Citation represents a reference to a source document
type Citation struct {
	Type          string `json:"type"` // char_location, page_location, content_block_location
	CitedText     string `json:"cited_text"`
	DocumentIndex int    `json:"document_index"`
	DocumentTitle string `json:"document_title,omitempty"`
	// For char_location (plain text)
	StartCharIndex *int `json:"start_char_index,omitempty"`
	EndCharIndex   *int `json:"end_char_index,omitempty"`
	// For page_location (PDF)
	StartPageNumber *int `json:"start_page_number,omitempty"`
	EndPageNumber   *int `json:"end_page_number,omitempty"`
	// For content_block_location (custom content)
	StartBlockIndex *int `json:"start_block_index,omitempty"`
	EndBlockIndex   *int `json:"end_block_index,omitempty"`
}

// ContentBlock represents a piece of content that may have citations
type ContentBlock struct {
	Type      string     `json:"type"` // text
	Text      string     `json:"text"`
	Citations []Citation `json:"citations,omitempty"`
}

type Metrics struct {
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	TotalTokens         int     `json:"total_tokens"`
	CacheCreationTokens int     `json:"cache_creation_input_tokens,omitempty"`
	CacheReadTokens     int     `json:"cache_read_input_tokens,omitempty"`
	TimeToFirstByte     float64 `json:"ttfb_ms"`
	TotalLatency        float64 `json:"total_latency_ms"`
	TokensPerSecond     float64 `json:"tokens_per_second"`
}

// API Request/Response types

type CreateConversationRequest struct {
	Title        string                `json:"title,omitempty"`
	Provider     string                `json:"provider"`
	Model        string                `json:"model"`
	SystemPrompt string                `json:"system_prompt,omitempty"`
	Settings     *ConversationSettings `json:"settings,omitempty"`
}

type UpdateConversationRequest struct {
	Title        *string               `json:"title,omitempty"`
	Model        *string               `json:"model,omitempty"`
	SystemPrompt *string               `json:"system_prompt,omitempty"`
	Settings     *ConversationSettings `json:"settings,omitempty"`
}

type SendMessageRequest struct {
	Content     string   `json:"content"`
	Attachments []string `json:"attachments,omitempty"` // attachment IDs
	ParentID    *string  `json:"parent_id,omitempty"`   // for forking
}

type RegenerateRequest struct {
	MessageID string `json:"message_id"`
}

type CompareRequest struct {
	Content   string              `json:"content"`
	Providers []ProviderSelection `json:"providers"`
}

type ProviderSelection struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// SSE Event types

type StreamEvent struct {
	Type      string      `json:"type"` // start, delta, metrics, done, error, citation
	Content   string      `json:"content,omitempty"`
	Metrics   *Metrics    `json:"metrics,omitempty"`
	Error     string      `json:"error,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Citations []Citation  `json:"citations,omitempty"` // For citations in text blocks
}

// ProviderInfo is defined in registry.go with extended fields

type PromptTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}
