package provider

import (
	"context"
	"testing"

	"github.com/spetr/chatapp/internal/models"
)

// MockProvider implements Provider interface for testing
type MockProvider struct {
	name   string
	models []string
}

func NewMockProvider(name string, models []string) *MockProvider {
	return &MockProvider{name: name, models: models}
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) Models() []string {
	return m.models
}

func (m *MockProvider) Chat(ctx context.Context, messages []models.Message, model string, systemPrompt string, opts *ChatOptions, callback StreamCallback) error {
	callback(models.StreamEvent{Type: "start"})
	callback(models.StreamEvent{Type: "delta", Content: "Mock response"})
	callback(models.StreamEvent{Type: "done"})
	return nil
}

func (m *MockProvider) ChatWithTools(ctx context.Context, messages []models.Message, model string, systemPrompt string, tools []Tool, opts *ChatOptions, callback StreamCallback) error {
	return m.Chat(ctx, messages, model, systemPrompt, opts, callback)
}

func (m *MockProvider) CountTokens(messages []models.Message) (int, error) {
	count := 0
	for _, msg := range messages {
		count += len(msg.Content) / 4 // Rough approximation
	}
	return count, nil
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	if registry == nil {
		t.Fatal("Expected registry to be created")
	}

	if len(registry.providers) != 0 {
		t.Errorf("Expected empty registry, got %d providers", len(registry.providers))
	}
}

func TestRegistryRegister(t *testing.T) {
	registry := NewRegistry()

	mock := NewMockProvider("test", []string{"model-1", "model-2"})
	registry.Register("test", mock)

	if len(registry.providers) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(registry.providers))
	}

	provider, ok := registry.Get("test")
	if !ok {
		t.Fatal("Expected provider to be found")
	}

	if provider.Name() != "test" {
		t.Errorf("Expected name 'test', got '%s'", provider.Name())
	}
}

func TestRegistryGet(t *testing.T) {
	registry := NewRegistry()

	mock := NewMockProvider("claude", []string{"claude-sonnet-4-20250514"})
	registry.Register("claude", mock)

	// Get existing
	provider, ok := registry.Get("claude")
	if !ok {
		t.Fatal("Expected provider to be found")
	}
	if provider == nil {
		t.Fatal("Expected provider to not be nil")
	}

	// Get non-existing
	_, ok = registry.Get("nonexistent")
	if ok {
		t.Error("Expected provider to not be found")
	}
}

func TestRegistryList(t *testing.T) {
	registry := NewRegistry()

	registry.Register("claude", NewMockProvider("claude", []string{"model-1"}))
	registry.Register("openai", NewMockProvider("openai", []string{"model-2"}))

	names := registry.List()
	if len(names) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(names))
	}

	// Check both are present (order not guaranteed)
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}

	if !found["claude"] {
		t.Error("Expected 'claude' in list")
	}
	if !found["openai"] {
		t.Error("Expected 'openai' in list")
	}
}

func TestRegistryAll(t *testing.T) {
	registry := NewRegistry()

	registry.Register("claude", NewMockProvider("claude", []string{"model-1"}))
	registry.Register("openai", NewMockProvider("openai", []string{"model-2"}))

	all := registry.All()
	if len(all) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(all))
	}

	if _, ok := all["claude"]; !ok {
		t.Error("Expected 'claude' in all")
	}
	if _, ok := all["openai"]; !ok {
		t.Error("Expected 'openai' in all")
	}
}

func TestMockProviderChat(t *testing.T) {
	mock := NewMockProvider("test", []string{"model-1"})

	var events []models.StreamEvent
	callback := func(event models.StreamEvent) {
		events = append(events, event)
	}

	err := mock.Chat(context.Background(), nil, "model-1", "", nil, callback)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}

	if events[0].Type != "start" {
		t.Errorf("Expected first event type 'start', got '%s'", events[0].Type)
	}
	if events[1].Type != "delta" {
		t.Errorf("Expected second event type 'delta', got '%s'", events[1].Type)
	}
	if events[2].Type != "done" {
		t.Errorf("Expected third event type 'done', got '%s'", events[2].Type)
	}
}

func TestMockProviderCountTokens(t *testing.T) {
	mock := NewMockProvider("test", []string{"model-1"})

	messages := []models.Message{
		{Content: "Hello"},        // 5 chars = ~1 token
		{Content: "How are you?"}, // 12 chars = ~3 tokens
	}

	count, err := mock.CountTokens(messages)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Our mock uses len/4, so 17 chars / 4 = 4 tokens
	if count != 4 {
		t.Errorf("Expected ~4 tokens, got %d", count)
	}
}

func TestToolStruct(t *testing.T) {
	tool := Tool{
		Name:        "get_weather",
		Description: "Get current weather",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]interface{}{
					"type":        "string",
					"description": "City name",
				},
			},
			"required": []string{"location"},
		},
	}

	if tool.Name != "get_weather" {
		t.Errorf("Expected name 'get_weather', got '%s'", tool.Name)
	}
	if tool.Description != "Get current weather" {
		t.Errorf("Expected description, got '%s'", tool.Description)
	}
	if tool.InputSchema == nil {
		t.Error("Expected input schema")
	}
}

func TestToolCallStruct(t *testing.T) {
	call := ToolCall{
		ID:   "call-123",
		Name: "get_weather",
		Input: map[string]interface{}{
			"location": "Prague",
		},
	}

	if call.ID != "call-123" {
		t.Errorf("Expected ID 'call-123', got '%s'", call.ID)
	}
	if call.Name != "get_weather" {
		t.Errorf("Expected name 'get_weather', got '%s'", call.Name)
	}
	if call.Input["location"] != "Prague" {
		t.Errorf("Expected location 'Prague', got '%v'", call.Input["location"])
	}
}

func TestToolResultStruct(t *testing.T) {
	result := ToolResult{
		ToolCallID: "call-123",
		Content:    "Temperature: 20°C",
		IsError:    false,
	}

	if result.ToolCallID != "call-123" {
		t.Errorf("Expected ToolCallID 'call-123', got '%s'", result.ToolCallID)
	}
	if result.Content != "Temperature: 20°C" {
		t.Errorf("Expected content, got '%s'", result.Content)
	}
	if result.IsError {
		t.Error("Expected IsError to be false")
	}

	// Test error result
	errorResult := ToolResult{
		ToolCallID: "call-456",
		Content:    "Location not found",
		IsError:    true,
	}

	if !errorResult.IsError {
		t.Error("Expected IsError to be true")
	}
}

func TestRegistryOverwrite(t *testing.T) {
	registry := NewRegistry()

	// Register first version
	mock1 := NewMockProvider("test", []string{"model-1"})
	registry.Register("test", mock1)

	// Overwrite with second version
	mock2 := NewMockProvider("test", []string{"model-2", "model-3"})
	registry.Register("test", mock2)

	provider, _ := registry.Get("test")
	models := provider.Models()

	if len(models) != 2 {
		t.Errorf("Expected 2 models after overwrite, got %d", len(models))
	}
}
