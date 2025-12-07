package provider

import (
	"context"

	"github.com/spetr/chatapp/internal/models"
)

// StreamCallback is called for each chunk of the response
type StreamCallback func(event models.StreamEvent)

// ChatOptions contains optional settings for chat requests
type ChatOptions struct {
	EnableThinking  bool
	EnableTools     bool
	EnableCitations bool // Enable document citations (Claude)
	Temperature     *float64
	MaxTokens       *int
	TopP            *float64
	TopK            *int
	Seed            *int
	ThinkingBudget  string // "low", "medium", "high" for Ollama GPT-OSS; token count for Claude
	Grammar         string // GBNF grammar for constrained generation (llama.cpp)
}

// Provider defines the interface for LLM providers
type Provider interface {
	// Name returns the provider identifier
	Name() string

	// Models returns available models for this provider
	Models() []string

	// Chat sends a message and streams the response
	Chat(ctx context.Context, messages []models.Message, model string, systemPrompt string, opts *ChatOptions, callback StreamCallback) error

	// ChatWithTools sends a message with MCP tools available
	ChatWithTools(ctx context.Context, messages []models.Message, model string, systemPrompt string, tools []Tool, opts *ChatOptions, callback StreamCallback) error

	// CountTokens estimates token count for messages
	CountTokens(messages []models.Message) (int, error)
}

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ToolCall represents a tool invocation by the LLM
type ToolCall struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// ToolResult represents the result of a tool call
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// Registry manages available providers
type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

func (r *Registry) Register(name string, provider Provider) {
	r.providers[name] = provider
}

func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

func (r *Registry) All() map[string]Provider {
	return r.providers
}
