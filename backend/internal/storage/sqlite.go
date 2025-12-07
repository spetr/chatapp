package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spetr/chatapp/internal/models"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type SQLiteStorage struct {
	db *sql.DB
}

func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	storage := &SQLiteStorage{db: db}
	if err := storage.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return storage, nil
}

func (s *SQLiteStorage) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			system_prompt TEXT,
			settings TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			metrics TEXT,
			parent_id TEXT,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS attachments (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			filename TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			path TEXT NOT NULL,
			data TEXT,
			FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_message ON attachments(message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_conversations_updated ON conversations(updated_at DESC)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Add settings column if it doesn't exist (for existing databases)
	s.db.Exec(`ALTER TABLE conversations ADD COLUMN settings TEXT`)

	// Add tool_calls column if it doesn't exist (for existing databases)
	s.db.Exec(`ALTER TABLE messages ADD COLUMN tool_calls TEXT`)

	return nil
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// Conversations

func (s *SQLiteStorage) CreateConversation(conv *models.Conversation) error {
	if conv.ID == "" {
		conv.ID = uuid.New().String()
	}
	now := time.Now()
	conv.CreatedAt = now
	conv.UpdatedAt = now

	var settingsJSON []byte
	if conv.Settings != nil {
		var err error
		settingsJSON, err = json.Marshal(conv.Settings)
		if err != nil {
			return err
		}
	}

	_, err := s.db.Exec(
		`INSERT INTO conversations (id, title, provider, model, system_prompt, settings, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		conv.ID, conv.Title, conv.Provider, conv.Model, conv.SystemPrompt, settingsJSON, conv.CreatedAt, conv.UpdatedAt,
	)
	return err
}

func (s *SQLiteStorage) GetConversation(id string) (*models.Conversation, error) {
	var conv models.Conversation
	var settingsJSON sql.NullString

	err := s.db.QueryRow(
		`SELECT id, title, provider, model, system_prompt, settings, created_at, updated_at
		FROM conversations WHERE id = ?`,
		id,
	).Scan(&conv.ID, &conv.Title, &conv.Provider, &conv.Model, &conv.SystemPrompt, &settingsJSON, &conv.CreatedAt, &conv.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if settingsJSON.Valid && settingsJSON.String != "" {
		conv.Settings = &models.ConversationSettings{}
		if err := json.Unmarshal([]byte(settingsJSON.String), conv.Settings); err != nil {
			conv.Settings = nil // Reset if parsing fails
		}
	}

	return &conv, nil
}

func (s *SQLiteStorage) ListConversations(limit, offset int) ([]models.Conversation, error) {
	rows, err := s.db.Query(
		`SELECT id, title, provider, model, system_prompt, settings, created_at, updated_at
		FROM conversations ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []models.Conversation
	for rows.Next() {
		var conv models.Conversation
		var settingsJSON sql.NullString

		if err := rows.Scan(&conv.ID, &conv.Title, &conv.Provider, &conv.Model, &conv.SystemPrompt, &settingsJSON, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
			return nil, err
		}

		if settingsJSON.Valid && settingsJSON.String != "" {
			conv.Settings = &models.ConversationSettings{}
			if err := json.Unmarshal([]byte(settingsJSON.String), conv.Settings); err != nil {
				conv.Settings = nil
			}
		}

		conversations = append(conversations, conv)
	}
	return conversations, nil
}

func (s *SQLiteStorage) UpdateConversation(conv *models.Conversation) error {
	conv.UpdatedAt = time.Now()

	var settingsJSON []byte
	if conv.Settings != nil {
		var err error
		settingsJSON, err = json.Marshal(conv.Settings)
		if err != nil {
			return err
		}
	}

	_, err := s.db.Exec(
		`UPDATE conversations SET title = ?, provider = ?, model = ?, system_prompt = ?, settings = ?, updated_at = ?
		WHERE id = ?`,
		conv.Title, conv.Provider, conv.Model, conv.SystemPrompt, settingsJSON, conv.UpdatedAt, conv.ID,
	)
	return err
}

func (s *SQLiteStorage) DeleteConversation(id string) error {
	_, err := s.db.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	return err
}

// Messages

func (s *SQLiteStorage) CreateMessage(msg *models.Message) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	msg.CreatedAt = time.Now()

	var metricsJSON []byte
	if msg.Metrics != nil {
		var err error
		metricsJSON, err = json.Marshal(msg.Metrics)
		if err != nil {
			return err
		}
	}

	var toolCallsJSON []byte
	if len(msg.ToolCalls) > 0 {
		var err error
		toolCallsJSON, err = json.Marshal(msg.ToolCalls)
		if err != nil {
			return err
		}
	}

	_, err := s.db.Exec(
		`INSERT INTO messages (id, conversation_id, role, content, metrics, parent_id, tool_calls, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ConversationID, msg.Role, msg.Content, metricsJSON, msg.ParentID, toolCallsJSON, msg.CreatedAt,
	)
	if err != nil {
		return err
	}

	// Save attachments
	for i := range msg.Attachments {
		msg.Attachments[i].MessageID = msg.ID
		if err := s.CreateAttachment(&msg.Attachments[i]); err != nil {
			return err
		}
	}

	// Update conversation timestamp
	s.db.Exec(`UPDATE conversations SET updated_at = ? WHERE id = ?`, time.Now(), msg.ConversationID)

	return nil
}

func (s *SQLiteStorage) GetMessage(id string) (*models.Message, error) {
	var msg models.Message
	var metricsJSON sql.NullString
	var parentID sql.NullString
	var toolCallsJSON sql.NullString

	err := s.db.QueryRow(
		`SELECT id, conversation_id, role, content, metrics, parent_id, tool_calls, created_at
		FROM messages WHERE id = ?`,
		id,
	).Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &metricsJSON, &parentID, &toolCallsJSON, &msg.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if metricsJSON.Valid && metricsJSON.String != "" {
		if err := json.Unmarshal([]byte(metricsJSON.String), &msg.Metrics); err != nil {
			return nil, err
		}
	}

	if parentID.Valid {
		msg.ParentID = &parentID.String
	}

	if toolCallsJSON.Valid && toolCallsJSON.String != "" {
		if err := json.Unmarshal([]byte(toolCallsJSON.String), &msg.ToolCalls); err != nil {
			return nil, err
		}
	}

	// Load attachments
	attachments, err := s.GetMessageAttachments(msg.ID)
	if err != nil {
		return nil, err
	}
	msg.Attachments = attachments

	return &msg, nil
}

func (s *SQLiteStorage) GetConversationMessages(conversationID string, parentID *string) ([]models.Message, error) {
	var rows *sql.Rows
	var err error

	if parentID == nil {
		rows, err = s.db.Query(
			`SELECT id, conversation_id, role, content, metrics, parent_id, tool_calls, created_at
			FROM messages WHERE conversation_id = ? ORDER BY created_at ASC`,
			conversationID,
		)
	} else {
		// Get messages in a fork chain
		rows, err = s.db.Query(
			`WITH RECURSIVE chain AS (
				SELECT * FROM messages WHERE id = ?
				UNION ALL
				SELECT m.* FROM messages m JOIN chain c ON m.parent_id = c.id
			)
			SELECT id, conversation_id, role, content, metrics, parent_id, tool_calls, created_at
			FROM chain ORDER BY created_at ASC`,
			*parentID,
		)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var msg models.Message
		var metricsJSON sql.NullString
		var pID sql.NullString
		var toolCallsJSON sql.NullString

		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &metricsJSON, &pID, &toolCallsJSON, &msg.CreatedAt); err != nil {
			return nil, err
		}

		if metricsJSON.Valid && metricsJSON.String != "" {
			json.Unmarshal([]byte(metricsJSON.String), &msg.Metrics)
		}

		if pID.Valid {
			msg.ParentID = &pID.String
		}

		if toolCallsJSON.Valid && toolCallsJSON.String != "" {
			json.Unmarshal([]byte(toolCallsJSON.String), &msg.ToolCalls)
		}

		// Load attachments
		attachments, _ := s.GetMessageAttachments(msg.ID)
		msg.Attachments = attachments

		messages = append(messages, msg)
	}

	return messages, nil
}

func (s *SQLiteStorage) UpdateMessage(msg *models.Message) error {
	var metricsJSON []byte
	if msg.Metrics != nil {
		var err error
		metricsJSON, err = json.Marshal(msg.Metrics)
		if err != nil {
			return err
		}
	}

	_, err := s.db.Exec(
		`UPDATE messages SET content = ?, metrics = ? WHERE id = ?`,
		msg.Content, metricsJSON, msg.ID,
	)
	return err
}

func (s *SQLiteStorage) DeleteMessage(id string) error {
	_, err := s.db.Exec(`DELETE FROM messages WHERE id = ?`, id)
	return err
}

// Attachments

func (s *SQLiteStorage) CreateAttachment(att *models.Attachment) error {
	if att.ID == "" {
		att.ID = uuid.New().String()
	}

	_, err := s.db.Exec(
		`INSERT INTO attachments (id, message_id, filename, mime_type, size, path, data)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		att.ID, att.MessageID, att.Filename, att.MimeType, att.Size, att.Path, att.Data,
	)
	return err
}

func (s *SQLiteStorage) GetAttachment(id string) (*models.Attachment, error) {
	var att models.Attachment
	var data sql.NullString

	err := s.db.QueryRow(
		`SELECT id, message_id, filename, mime_type, size, path, data
		FROM attachments WHERE id = ?`,
		id,
	).Scan(&att.ID, &att.MessageID, &att.Filename, &att.MimeType, &att.Size, &att.Path, &data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if data.Valid {
		att.Data = data.String
	}

	return &att, nil
}

func (s *SQLiteStorage) GetMessageAttachments(messageID string) ([]models.Attachment, error) {
	rows, err := s.db.Query(
		`SELECT id, message_id, filename, mime_type, size, path, data
		FROM attachments WHERE message_id = ?`,
		messageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attachments []models.Attachment
	for rows.Next() {
		var att models.Attachment
		var data sql.NullString

		if err := rows.Scan(&att.ID, &att.MessageID, &att.Filename, &att.MimeType, &att.Size, &att.Path, &data); err != nil {
			return nil, err
		}

		if data.Valid {
			att.Data = data.String
		}

		attachments = append(attachments, att)
	}

	return attachments, nil
}

func (s *SQLiteStorage) DeleteAttachment(id string) error {
	_, err := s.db.Exec(`DELETE FROM attachments WHERE id = ?`, id)
	return err
}
