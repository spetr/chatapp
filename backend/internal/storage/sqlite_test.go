package storage

import (
	"github.com/spetr/chatapp/internal/models"
	"path/filepath"
	"testing"
)

func TestNewSQLiteStorage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	if storage.db == nil {
		t.Error("Expected db to be initialized")
	}
}

func TestConversationCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create
	conv := &models.Conversation{
		Title:        "Test Conversation",
		Provider:     "claude",
		Model:        "claude-sonnet-4-20250514",
		SystemPrompt: "You are a helpful assistant",
	}

	if err := storage.CreateConversation(conv); err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	if conv.ID == "" {
		t.Error("Expected conversation ID to be set")
	}

	// Read
	loaded, err := storage.GetConversation(conv.ID)
	if err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}

	if loaded == nil {
		t.Fatal("Expected conversation to be found")
	}

	if loaded.Title != "Test Conversation" {
		t.Errorf("Expected title 'Test Conversation', got '%s'", loaded.Title)
	}

	if loaded.Provider != "claude" {
		t.Errorf("Expected provider 'claude', got '%s'", loaded.Provider)
	}

	// Update
	loaded.Title = "Updated Title"
	if err := storage.UpdateConversation(loaded); err != nil {
		t.Fatalf("Failed to update conversation: %v", err)
	}

	updated, _ := storage.GetConversation(conv.ID)
	if updated.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", updated.Title)
	}

	// List
	convs, err := storage.ListConversations(10, 0)
	if err != nil {
		t.Fatalf("Failed to list conversations: %v", err)
	}

	if len(convs) != 1 {
		t.Errorf("Expected 1 conversation, got %d", len(convs))
	}

	// Delete
	if err := storage.DeleteConversation(conv.ID); err != nil {
		t.Fatalf("Failed to delete conversation: %v", err)
	}

	deleted, _ := storage.GetConversation(conv.ID)
	if deleted != nil {
		t.Error("Expected conversation to be deleted")
	}
}

func TestMessageCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create conversation first
	conv := &models.Conversation{
		Title:    "Test",
		Provider: "claude",
		Model:    "claude-sonnet-4-20250514",
	}
	storage.CreateConversation(conv)

	// Create message
	msg := &models.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Hello, world!",
		Metrics: &models.Metrics{
			InputTokens:  10,
			OutputTokens: 0,
			TotalTokens:  10,
		},
	}

	if err := storage.CreateMessage(msg); err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	if msg.ID == "" {
		t.Error("Expected message ID to be set")
	}

	// Read
	loaded, err := storage.GetMessage(msg.ID)
	if err != nil {
		t.Fatalf("Failed to get message: %v", err)
	}

	if loaded == nil {
		t.Fatal("Expected message to be found")
	}

	if loaded.Content != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got '%s'", loaded.Content)
	}

	if loaded.Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", loaded.Role)
	}

	if loaded.Metrics == nil {
		t.Error("Expected metrics to be loaded")
	} else if loaded.Metrics.InputTokens != 10 {
		t.Errorf("Expected input tokens 10, got %d", loaded.Metrics.InputTokens)
	}

	// Get conversation messages
	msgs, err := storage.GetConversationMessages(conv.ID, nil)
	if err != nil {
		t.Fatalf("Failed to get conversation messages: %v", err)
	}

	if len(msgs) != 1 {
		t.Errorf("Expected 1 message, got %d", len(msgs))
	}

	// Update
	loaded.Content = "Updated content"
	if err := storage.UpdateMessage(loaded); err != nil {
		t.Fatalf("Failed to update message: %v", err)
	}

	updated, _ := storage.GetMessage(msg.ID)
	if updated.Content != "Updated content" {
		t.Errorf("Expected content 'Updated content', got '%s'", updated.Content)
	}

	// Delete
	if err := storage.DeleteMessage(msg.ID); err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	deleted, _ := storage.GetMessage(msg.ID)
	if deleted != nil {
		t.Error("Expected message to be deleted")
	}
}

func TestAttachmentCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create conversation and message first
	conv := &models.Conversation{
		Title:    "Test",
		Provider: "claude",
		Model:    "claude-sonnet-4-20250514",
	}
	storage.CreateConversation(conv)

	msg := &models.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Test message",
	}
	storage.CreateMessage(msg)

	// Create attachment
	att := &models.Attachment{
		MessageID: msg.ID,
		Filename:  "test.txt",
		MimeType:  "text/plain",
		Size:      100,
		Path:      "/tmp/test.txt",
		Data:      "SGVsbG8gV29ybGQ=", // base64 "Hello World"
	}

	if err := storage.CreateAttachment(att); err != nil {
		t.Fatalf("Failed to create attachment: %v", err)
	}

	if att.ID == "" {
		t.Error("Expected attachment ID to be set")
	}

	// Read
	loaded, err := storage.GetAttachment(att.ID)
	if err != nil {
		t.Fatalf("Failed to get attachment: %v", err)
	}

	if loaded == nil {
		t.Fatal("Expected attachment to be found")
	}

	if loaded.Filename != "test.txt" {
		t.Errorf("Expected filename 'test.txt', got '%s'", loaded.Filename)
	}

	if loaded.MimeType != "text/plain" {
		t.Errorf("Expected mime type 'text/plain', got '%s'", loaded.MimeType)
	}

	// Get message attachments
	atts, err := storage.GetMessageAttachments(msg.ID)
	if err != nil {
		t.Fatalf("Failed to get message attachments: %v", err)
	}

	if len(atts) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(atts))
	}

	// Delete
	if err := storage.DeleteAttachment(att.ID); err != nil {
		t.Fatalf("Failed to delete attachment: %v", err)
	}

	deleted, _ := storage.GetAttachment(att.ID)
	if deleted != nil {
		t.Error("Expected attachment to be deleted")
	}
}

func TestCascadeDelete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create conversation with messages and attachments
	conv := &models.Conversation{
		Title:    "Test",
		Provider: "claude",
		Model:    "claude-sonnet-4-20250514",
	}
	storage.CreateConversation(conv)

	msg := &models.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Test message",
	}
	storage.CreateMessage(msg)

	att := &models.Attachment{
		MessageID: msg.ID,
		Filename:  "test.txt",
		MimeType:  "text/plain",
		Size:      100,
		Path:      "/tmp/test.txt",
	}
	storage.CreateAttachment(att)

	// Delete conversation - should cascade to messages and attachments
	if err := storage.DeleteConversation(conv.ID); err != nil {
		t.Fatalf("Failed to delete conversation: %v", err)
	}

	// Check message is deleted
	deletedMsg, _ := storage.GetMessage(msg.ID)
	if deletedMsg != nil {
		t.Error("Expected message to be deleted via cascade")
	}

	// Check attachment is deleted
	deletedAtt, _ := storage.GetAttachment(att.ID)
	if deletedAtt != nil {
		t.Error("Expected attachment to be deleted via cascade")
	}
}

func TestGetNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Get nonexistent conversation
	conv, err := storage.GetConversation("nonexistent-id")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if conv != nil {
		t.Error("Expected nil for nonexistent conversation")
	}

	// Get nonexistent message
	msg, err := storage.GetMessage("nonexistent-id")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if msg != nil {
		t.Error("Expected nil for nonexistent message")
	}

	// Get nonexistent attachment
	att, err := storage.GetAttachment("nonexistent-id")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if att != nil {
		t.Error("Expected nil for nonexistent attachment")
	}
}

func TestMultipleConversations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create multiple conversations
	for i := 0; i < 5; i++ {
		conv := &models.Conversation{
			Title:    "Test",
			Provider: "claude",
			Model:    "claude-sonnet-4-20250514",
		}
		storage.CreateConversation(conv)
	}

	// Test limit
	convs, err := storage.ListConversations(3, 0)
	if err != nil {
		t.Fatalf("Failed to list conversations: %v", err)
	}
	if len(convs) != 3 {
		t.Errorf("Expected 3 conversations with limit, got %d", len(convs))
	}

	// Test offset
	convs, err = storage.ListConversations(10, 2)
	if err != nil {
		t.Fatalf("Failed to list conversations: %v", err)
	}
	if len(convs) != 3 {
		t.Errorf("Expected 3 conversations with offset, got %d", len(convs))
	}
}
