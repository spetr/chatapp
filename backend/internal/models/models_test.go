package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConversationJSON(t *testing.T) {
	conv := Conversation{
		ID:           "test-id",
		Title:        "Test Conversation",
		Provider:     "claude",
		Model:        "claude-sonnet-4-20250514",
		SystemPrompt: "You are helpful",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Marshal
	data, err := json.Marshal(conv)
	if err != nil {
		t.Fatalf("Failed to marshal conversation: %v", err)
	}

	// Unmarshal
	var loaded Conversation
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal conversation: %v", err)
	}

	if loaded.ID != conv.ID {
		t.Errorf("Expected ID %s, got %s", conv.ID, loaded.ID)
	}
	if loaded.Title != conv.Title {
		t.Errorf("Expected Title %s, got %s", conv.Title, loaded.Title)
	}
}

func TestMessageJSON(t *testing.T) {
	parentID := "parent-123"
	msg := Message{
		ID:             "msg-id",
		ConversationID: "conv-id",
		Role:           "assistant",
		Content:        "Hello!",
		Metrics: &Metrics{
			InputTokens:     100,
			OutputTokens:    50,
			TotalTokens:     150,
			TimeToFirstByte: 250.5,
			TotalLatency:    1000.0,
			TokensPerSecond: 50.0,
		},
		ParentID:  &parentID,
		CreatedAt: time.Now(),
	}

	// Marshal
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	// Unmarshal
	var loaded Message
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if loaded.ID != msg.ID {
		t.Errorf("Expected ID %s, got %s", msg.ID, loaded.ID)
	}
	if loaded.Role != msg.Role {
		t.Errorf("Expected Role %s, got %s", msg.Role, loaded.Role)
	}
	if loaded.Metrics == nil {
		t.Fatal("Expected metrics to be loaded")
	}
	if loaded.Metrics.InputTokens != 100 {
		t.Errorf("Expected InputTokens 100, got %d", loaded.Metrics.InputTokens)
	}
	if loaded.ParentID == nil || *loaded.ParentID != parentID {
		t.Error("Expected ParentID to be set")
	}
}

func TestMessageJSONOmitEmpty(t *testing.T) {
	msg := Message{
		ID:             "msg-id",
		ConversationID: "conv-id",
		Role:           "user",
		Content:        "Hello",
		CreatedAt:      time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	str := string(data)
	if containsString(str, "attachments") {
		t.Error("Expected attachments to be omitted when empty")
	}
	if containsString(str, "metrics") {
		t.Error("Expected metrics to be omitted when nil")
	}
	if containsString(str, "parent_id") {
		t.Error("Expected parent_id to be omitted when nil")
	}
}

func TestAttachmentJSON(t *testing.T) {
	att := Attachment{
		ID:        "att-id",
		MessageID: "msg-id",
		Filename:  "test.png",
		MimeType:  "image/png",
		Size:      1024,
		Path:      "/uploads/test.png",
		Data:      "base64data...",
	}

	data, err := json.Marshal(att)
	if err != nil {
		t.Fatalf("Failed to marshal attachment: %v", err)
	}

	var loaded Attachment
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal attachment: %v", err)
	}

	if loaded.Filename != att.Filename {
		t.Errorf("Expected Filename %s, got %s", att.Filename, loaded.Filename)
	}
	if loaded.Size != att.Size {
		t.Errorf("Expected Size %d, got %d", att.Size, loaded.Size)
	}
}

func TestMetricsJSON(t *testing.T) {
	metrics := Metrics{
		InputTokens:     500,
		OutputTokens:    200,
		TotalTokens:     700,
		TimeToFirstByte: 150.5,
		TotalLatency:    2500.0,
		TokensPerSecond: 80.0,
	}

	data, err := json.Marshal(metrics)
	if err != nil {
		t.Fatalf("Failed to marshal metrics: %v", err)
	}

	var loaded Metrics
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal metrics: %v", err)
	}

	if loaded.InputTokens != 500 {
		t.Errorf("Expected InputTokens 500, got %d", loaded.InputTokens)
	}
	if loaded.TokensPerSecond != 80.0 {
		t.Errorf("Expected TokensPerSecond 80.0, got %f", loaded.TokensPerSecond)
	}
}

func TestStreamEventJSON(t *testing.T) {
	// Test delta event
	event := StreamEvent{
		Type:    "delta",
		Content: "Hello ",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var loaded StreamEvent
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if loaded.Type != "delta" {
		t.Errorf("Expected Type 'delta', got '%s'", loaded.Type)
	}
	if loaded.Content != "Hello " {
		t.Errorf("Expected Content 'Hello ', got '%s'", loaded.Content)
	}

	// Test metrics event
	metricsEvent := StreamEvent{
		Type: "metrics",
		Metrics: &Metrics{
			TotalTokens: 100,
		},
	}

	data, err = json.Marshal(metricsEvent)
	if err != nil {
		t.Fatalf("Failed to marshal metrics event: %v", err)
	}

	var loadedMetrics StreamEvent
	if err := json.Unmarshal(data, &loadedMetrics); err != nil {
		t.Fatalf("Failed to unmarshal metrics event: %v", err)
	}

	if loadedMetrics.Metrics == nil {
		t.Fatal("Expected metrics to be present")
	}
	if loadedMetrics.Metrics.TotalTokens != 100 {
		t.Errorf("Expected TotalTokens 100, got %d", loadedMetrics.Metrics.TotalTokens)
	}
}

func TestCreateConversationRequest(t *testing.T) {
	req := CreateConversationRequest{
		Title:        "My Chat",
		Provider:     "openai",
		Model:        "gpt-4o",
		SystemPrompt: "Be helpful",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var loaded CreateConversationRequest
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if loaded.Provider != "openai" {
		t.Errorf("Expected Provider 'openai', got '%s'", loaded.Provider)
	}
}

func TestSendMessageRequest(t *testing.T) {
	parentID := "parent-123"
	req := SendMessageRequest{
		Content:     "Hello AI",
		Attachments: []string{"att-1", "att-2"},
		ParentID:    &parentID,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var loaded SendMessageRequest
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(loaded.Attachments) != 2 {
		t.Errorf("Expected 2 attachments, got %d", len(loaded.Attachments))
	}
	if loaded.ParentID == nil || *loaded.ParentID != parentID {
		t.Error("Expected ParentID to be set")
	}
}

func TestCompareRequest(t *testing.T) {
	req := CompareRequest{
		Content: "Compare this",
		Providers: []ProviderSelection{
			{Provider: "claude", Model: "claude-sonnet-4-20250514"},
			{Provider: "openai", Model: "gpt-4o"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var loaded CompareRequest
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(loaded.Providers) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(loaded.Providers))
	}
}

func TestProviderInfo(t *testing.T) {
	info := ProviderInfo{
		ID:          "claude",
		Name:        "Claude",
		Description: "Claude models from Anthropic",
		Type:        "cloud",
		Available:   true,
		HasAPIKey:   true,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var loaded ProviderInfo
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if loaded.ID != "claude" {
		t.Errorf("Expected ID 'claude', got '%s'", loaded.ID)
	}
	if loaded.Type != "cloud" {
		t.Errorf("Expected Type 'cloud', got '%s'", loaded.Type)
	}
}

func TestPromptTemplate(t *testing.T) {
	template := PromptTemplate{
		ID:          "coding",
		Name:        "Coding Assistant",
		Description: "Helps with programming",
		Content:     "You are an expert programmer...",
	}

	data, err := json.Marshal(template)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var loaded PromptTemplate
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if loaded.ID != "coding" {
		t.Errorf("Expected ID 'coding', got '%s'", loaded.ID)
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
