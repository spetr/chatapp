package context

import (
	"context"
	"fmt"
	"strings"

	"github.com/spetr/chatapp/internal/config"
	"github.com/spetr/chatapp/internal/models"
	"github.com/spetr/chatapp/internal/provider"
)

// Manager handles intelligent context management for conversations
type Manager struct {
	config   config.ContextConfig
	provider provider.Provider // For summarization
}

// Checkpoint represents a saved state of the conversation
type Checkpoint struct {
	ID           string `json:"id"`
	MessageIndex int    `json:"message_index"` // Index of last message in checkpoint
	Summary      string `json:"summary"`       // Summary of messages up to this point
	TokenCount   int    `json:"token_count"`   // Tokens in summary
}

// ProcessedContext is the result of context processing
type ProcessedContext struct {
	Messages        []models.Message `json:"messages"`
	SystemPrompt    string           `json:"system_prompt"`
	TotalTokens     int              `json:"total_tokens"`
	WasTruncated    bool             `json:"was_truncated"`
	WasSummarized   bool             `json:"was_summarized"`
	CacheBreakpoint int              `json:"cache_breakpoint"` // Index where cache should be set
}

func NewManager(cfg config.ContextConfig, prov provider.Provider) *Manager {
	return &Manager{
		config:   cfg,
		provider: prov,
	}
}

// ProcessContext takes raw messages and returns optimized context for the API
func (m *Manager) ProcessContext(messages []models.Message, systemPrompt string, checkpoint *Checkpoint) (*ProcessedContext, error) {
	result := &ProcessedContext{
		SystemPrompt: systemPrompt,
	}

	// If no messages, return empty
	if len(messages) == 0 {
		return result, nil
	}

	// Estimate current token count
	totalTokens := m.estimateTokens(systemPrompt)
	for _, msg := range messages {
		totalTokens += m.estimateMessageTokens(msg)
	}
	result.TotalTokens = totalTokens

	// Check if we need to process
	needsProcessing := false
	if m.config.MaxTokens > 0 && totalTokens > m.config.MaxTokens {
		needsProcessing = true
	}
	if m.config.MaxMessages > 0 && len(messages) > m.config.MaxMessages {
		needsProcessing = true
	}

	if !needsProcessing {
		result.Messages = messages
		result.CacheBreakpoint = len(messages) * 80 / 100 // Cache first 80%
		return result, nil
	}

	// Apply processing strategies
	processed := messages

	// Strategy 1: Apply checkpoint summary if available
	if checkpoint != nil && checkpoint.MessageIndex > 0 {
		// Start with summary, then messages after checkpoint
		summaryMsg := models.Message{
			Role:    "system",
			Content: fmt.Sprintf("[Previous conversation summary: %s]", checkpoint.Summary),
		}
		processed = append([]models.Message{summaryMsg}, messages[checkpoint.MessageIndex:]...)
		result.WasSummarized = true
	}

	// Strategy 2: Sliding window - keep most recent messages
	if m.config.MaxMessages > 0 && len(processed) > m.config.MaxMessages {
		// Keep first message (often important) + last N messages
		keepFirst := 1
		keepLast := m.config.MaxMessages - keepFirst - 1 // -1 for potential summary

		if len(processed) > keepFirst+keepLast {
			// Create summary of middle messages
			middleStart := keepFirst
			middleEnd := len(processed) - keepLast
			middleMessages := processed[middleStart:middleEnd]

			// Generate brief summary of middle section
			middleSummary := m.generateQuickSummary(middleMessages)

			summaryMsg := models.Message{
				Role:    "system",
				Content: fmt.Sprintf("[Summarized %d earlier messages: %s]", len(middleMessages), middleSummary),
			}

			newProcessed := make([]models.Message, 0, m.config.MaxMessages)
			newProcessed = append(newProcessed, processed[:keepFirst]...)
			newProcessed = append(newProcessed, summaryMsg)
			newProcessed = append(newProcessed, processed[middleEnd:]...)
			processed = newProcessed
			result.WasSummarized = true
		}

		result.WasTruncated = true
	}

	// Strategy 3: Truncate long individual messages
	if m.config.TruncateLongMsgs && m.config.MaxMsgLength > 0 {
		for i, msg := range processed {
			if len(msg.Content) > m.config.MaxMsgLength {
				// Keep beginning and end, truncate middle
				keepChars := m.config.MaxMsgLength / 2
				truncated := msg.Content[:keepChars] +
					"\n\n[... content truncated ...]\n\n" +
					msg.Content[len(msg.Content)-keepChars:]
				processed[i].Content = truncated
				result.WasTruncated = true
			}
		}
	}

	// Strategy 4: Token-based truncation as last resort
	if m.config.MaxTokens > 0 {
		processed = m.truncateToTokenLimit(processed, m.config.MaxTokens-m.estimateTokens(systemPrompt))
	}

	result.Messages = processed
	result.CacheBreakpoint = len(processed) * 80 / 100 // Cache first 80%

	// Recalculate total tokens
	result.TotalTokens = m.estimateTokens(systemPrompt)
	for _, msg := range processed {
		result.TotalTokens += m.estimateMessageTokens(msg)
	}

	return result, nil
}

// CreateCheckpoint creates a checkpoint from current conversation state
func (m *Manager) CreateCheckpoint(ctx context.Context, messages []models.Message, existingCheckpoint *Checkpoint) (*Checkpoint, error) {
	if len(messages) < 10 { // Don't checkpoint small conversations
		return nil, nil
	}

	// Determine what to summarize
	startIdx := 0
	if existingCheckpoint != nil {
		startIdx = existingCheckpoint.MessageIndex
	}

	// Only checkpoint if we have enough new messages
	if len(messages)-startIdx < 10 {
		return existingCheckpoint, nil
	}

	// Create summary of messages from startIdx to len-5 (keep recent messages unsummarized)
	endIdx := len(messages) - 5
	toSummarize := messages[startIdx:endIdx]

	summary := m.generateDetailedSummary(toSummarize)

	return &Checkpoint{
		ID:           fmt.Sprintf("cp_%d", endIdx),
		MessageIndex: endIdx,
		Summary:      summary,
		TokenCount:   m.estimateTokens(summary),
	}, nil
}

// estimateTokens provides a rough token count (4 chars ≈ 1 token for English)
func (m *Manager) estimateTokens(text string) int {
	return len(text) / 4
}

func (m *Manager) estimateMessageTokens(msg models.Message) int {
	tokens := m.estimateTokens(msg.Content)
	// Add overhead for role, formatting
	tokens += 10
	// Add for attachments
	for _, att := range msg.Attachments {
		if strings.HasPrefix(att.MimeType, "image/") {
			tokens += 1000 // Images cost more
		} else {
			tokens += m.estimateTokens(att.Filename) + 50
		}
	}
	return tokens
}

func (m *Manager) truncateToTokenLimit(messages []models.Message, maxTokens int) []models.Message {
	currentTokens := 0
	result := make([]models.Message, 0)

	// Work backwards from the end (keep most recent)
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := m.estimateMessageTokens(messages[i])
		if currentTokens+msgTokens > maxTokens {
			break
		}
		currentTokens += msgTokens
		result = append([]models.Message{messages[i]}, result...)
	}

	return result
}

// generateQuickSummary creates a brief summary without calling LLM
func (m *Manager) generateQuickSummary(messages []models.Message) string {
	if len(messages) == 0 {
		return ""
	}

	var topics []string
	for _, msg := range messages {
		// Extract first sentence or first 100 chars
		content := msg.Content
		if idx := strings.Index(content, ". "); idx > 0 && idx < 150 {
			content = content[:idx+1]
		} else if len(content) > 100 {
			content = content[:100] + "..."
		}

		if msg.Role == "user" {
			topics = append(topics, fmt.Sprintf("User asked: %s", content))
		} else if msg.Role == "assistant" {
			topics = append(topics, fmt.Sprintf("Assistant discussed: %s", content))
		}
	}

	// Keep only first and last few topics
	if len(topics) > 6 {
		topics = append(topics[:3], topics[len(topics)-3:]...)
	}

	return strings.Join(topics, " | ")
}

// generateDetailedSummary would ideally call LLM for better summary
// For now, creates a structured summary
func (m *Manager) generateDetailedSummary(messages []models.Message) string {
	var userTopics, assistantTopics []string

	for _, msg := range messages {
		// Extract key points
		sentences := strings.Split(msg.Content, ". ")
		if len(sentences) > 0 {
			firstSentence := sentences[0]
			if len(firstSentence) > 200 {
				firstSentence = firstSentence[:200]
			}

			if msg.Role == "user" {
				userTopics = append(userTopics, firstSentence)
			} else if msg.Role == "assistant" {
				assistantTopics = append(assistantTopics, firstSentence)
			}
		}
	}

	// Deduplicate and limit
	userTopics = uniqueStrings(userTopics)
	assistantTopics = uniqueStrings(assistantTopics)

	if len(userTopics) > 5 {
		userTopics = userTopics[:5]
	}
	if len(assistantTopics) > 5 {
		assistantTopics = assistantTopics[:5]
	}

	parts := []string{}
	if len(userTopics) > 0 {
		parts = append(parts, fmt.Sprintf("User discussed: %s", strings.Join(userTopics, "; ")))
	}
	if len(assistantTopics) > 0 {
		parts = append(parts, fmt.Sprintf("Assistant covered: %s", strings.Join(assistantTopics, "; ")))
	}

	return strings.Join(parts, " | ")
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, s := range input {
		if !seen[s] && len(s) > 10 {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// ShouldCreateCheckpoint determines if we should create a new checkpoint
func (m *Manager) ShouldCreateCheckpoint(messages []models.Message, existingCheckpoint *Checkpoint) bool {
	if len(messages) < 20 {
		return false
	}

	if existingCheckpoint == nil {
		return len(messages) >= 20
	}

	// Create new checkpoint every 15 messages
	return len(messages)-existingCheckpoint.MessageIndex >= 15
}

// GetContextStats returns statistics about the context
type ContextStats struct {
	MessageCount      int     `json:"message_count"`
	EstimatedTokens   int     `json:"estimated_tokens"`
	TokenPercentUsed  float64 `json:"token_percent_used"`
	NeedsOptimization bool    `json:"needs_optimization"`
	RecommendedAction string  `json:"recommended_action"`
}

func (m *Manager) GetContextStats(messages []models.Message, systemPrompt string) ContextStats {
	totalTokens := m.estimateTokens(systemPrompt)
	for _, msg := range messages {
		totalTokens += m.estimateMessageTokens(msg)
	}

	maxTokens := m.config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 100000 // Default assumption
	}

	percentUsed := float64(totalTokens) / float64(maxTokens) * 100
	needsOpt := percentUsed > 70 || (m.config.MaxMessages > 0 && len(messages) > m.config.MaxMessages*80/100)

	action := ""
	if percentUsed > 90 {
		action = "Critical: Start new conversation or enable summarization"
	} else if percentUsed > 70 {
		action = "Consider starting new conversation soon"
	} else if percentUsed > 50 {
		action = "Context growing, checkpoint recommended"
	}

	return ContextStats{
		MessageCount:      len(messages),
		EstimatedTokens:   totalTokens,
		TokenPercentUsed:  percentUsed,
		NeedsOptimization: needsOpt,
		RecommendedAction: action,
	}
}
