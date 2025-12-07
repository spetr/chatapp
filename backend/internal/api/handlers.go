package api

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/spetr/chatapp/internal/config"
	ctxmgr "github.com/spetr/chatapp/internal/context"
	"github.com/spetr/chatapp/internal/mcp"
	"github.com/spetr/chatapp/internal/models"
	"github.com/spetr/chatapp/internal/provider"
	"github.com/spetr/chatapp/internal/storage"
)

// Handler manages HTTP API endpoints for the chat application.
// It coordinates between storage, providers, and MCP tools.
type Handler struct {
	config     *config.Config
	configPath string
	storage    *storage.SQLiteStorage
	providers  *provider.Registry
	mcp        *mcp.Client
	configMu   sync.RWMutex // Protects config access

	// Stream cancellation management
	// activeStreams maps stream IDs to their cancel functions
	// allowing users to stop ongoing LLM generations
	activeStreams   map[string]context.CancelFunc
	activeStreamsMu sync.RWMutex // Protects activeStreams map access
}

func NewHandler(cfg *config.Config, configPath string, store *storage.SQLiteStorage, providers *provider.Registry, mcpClient *mcp.Client) *Handler {
	return &Handler{
		config:        cfg,
		configPath:    configPath,
		storage:       store,
		providers:     providers,
		mcp:           mcpClient,
		activeStreams: make(map[string]context.CancelFunc),
	}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")

	// Health
	api.Get("/health", h.Health)

	// Providers
	api.Get("/providers", h.ListProviders)
	api.Get("/models", h.ListModels)
	api.Get("/prompts", h.ListPrompts)

	// Conversations
	api.Get("/conversations", h.ListConversations)
	api.Post("/conversations", h.CreateConversation)
	api.Get("/conversations/:id", h.GetConversation)
	api.Put("/conversations/:id", h.UpdateConversation)
	api.Delete("/conversations/:id", h.DeleteConversation)
	api.Get("/conversations/:id/export", h.ExportConversation)

	// Messages
	api.Get("/conversations/:id/messages", h.GetMessages)
	api.Post("/conversations/:id/messages", h.SendMessage)
	api.Post("/conversations/:id/regenerate", h.RegenerateMessage)
	api.Post("/conversations/:id/stop", h.StopGeneration)

	// Compare
	api.Post("/compare", h.CompareProviders)

	// Files
	api.Post("/upload", h.UploadFile)
	api.Get("/attachments/:id", h.GetAttachment)

	// MCP
	api.Get("/mcp/tools", h.ListMCPTools)
	api.Get("/mcp/status", h.GetMCPStatus)

	// Context management
	api.Get("/conversations/:id/context-stats", h.GetContextStats)
	api.Get("/conversations/:id/context-breakdown", h.GetContextBreakdown)
	api.Post("/conversations/:id/context-compact", h.CompactContext)
	api.Get("/conversations/:id/context-preview", h.GetContextPreview)

	// Configuration
	api.Get("/config", h.GetConfig)
	api.Put("/config", h.UpdateConfig)
	api.Get("/config/path", h.GetConfigPath)

	// Ollama
	api.Get("/ollama/models", h.ListOllamaModels)
	api.Get("/ollama/gpus", h.GetGPUOptions)
	api.Get("/ollama/config", h.GetOllamaConfig)
	api.Put("/ollama/config", h.UpdateOllamaConfig)

	// OpenAI models
	api.Get("/openai/models", h.ListOpenAIModels)

	// llama.cpp
	api.Get("/llamacpp/health", h.GetLlamaCppHealth)
	api.Get("/llamacpp/props", h.GetLlamaCppProps)
	api.Get("/llamacpp/models", h.ListLlamaCppModels)
	api.Post("/llamacpp/infill", h.LlamaCppInfill)
	api.Post("/llamacpp/tokenize", h.LlamaCppTokenize)
	api.Post("/llamacpp/detokenize", h.LlamaCppDetokenize)
	api.Post("/llamacpp/embedding", h.LlamaCppEmbedding)

	// Pricing
	api.Get("/pricing", h.GetPricing)
}

// Health
func (h *Handler) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

// Providers
func (h *Handler) ListProviders(c *fiber.Ctx) error {
	providers := make([]models.ProviderInfo, 0)

	// Provider metadata
	providerMeta := map[string]struct {
		name        string
		description string
		provType    string
	}{
		"claude":   {"Claude", "Claude models from Anthropic", "cloud"},
		"openai":   {"OpenAI", "GPT models from OpenAI", "cloud"},
		"ollama":   {"Ollama", "Local models via Ollama", "local"},
		"llamacpp": {"llama.cpp", "Direct llama.cpp server connection", "local"},
	}

	for name, cfg := range h.config.Providers {
		meta, ok := providerMeta[name]
		if !ok {
			meta.name = strings.ToUpper(name[:1]) + name[1:]
			meta.provType = "cloud"
		}

		// Check availability
		available := false
		switch cfg.Type {
		case "anthropic", "openai":
			available = cfg.APIKey != ""
		case "ollama", "llamacpp":
			available = true // Local providers always "available" if configured
		}

		providers = append(providers, models.ProviderInfo{
			ID:          name,
			Name:        meta.name,
			Description: meta.description,
			Type:        meta.provType,
			Available:   available,
			HasAPIKey:   cfg.APIKey != "",
		})
	}

	return c.JSON(providers)
}

// ListModels returns all models from the registry
func (h *Handler) ListModels(c *fiber.Ctx) error {
	registry := models.GetRegistry()
	providerFilter := c.Query("provider")

	var result []*models.ModelInfo
	if providerFilter != "" {
		// Map config provider name to provider type
		// e.g., "claude" -> "anthropic"
		providerType := providerFilter
		if cfg, ok := h.config.Providers[providerFilter]; ok {
			providerType = cfg.Type
		}
		result = registry.GetByProvider(providerType)
	} else {
		result = registry.All()
	}

	// Sort by provider, then by display name
	sort.Slice(result, func(i, j int) bool {
		if result[i].Provider != result[j].Provider {
			return result[i].Provider < result[j].Provider
		}
		return result[i].DisplayName < result[j].DisplayName
	})

	return c.JSON(result)
}

func (h *Handler) ListPrompts(c *fiber.Ctx) error {
	prompts := make([]models.PromptTemplate, 0)

	for id, cfg := range h.config.Prompts {
		prompts = append(prompts, models.PromptTemplate{
			ID:          id,
			Name:        cfg.Name,
			Description: cfg.Description,
			Content:     cfg.Content,
		})
	}

	return c.JSON(prompts)
}

// Conversations
func (h *Handler) ListConversations(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	conversations, err := h.storage.ListConversations(limit, offset)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(conversations)
}

func (h *Handler) CreateConversation(c *fiber.Ctx) error {
	var req models.CreateConversationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	// Get system prompt
	systemPrompt := req.SystemPrompt
	if systemPrompt == "" {
		if prompt, ok := h.config.Prompts["default"]; ok {
			systemPrompt = prompt.Content
		}
	}

	conv := &models.Conversation{
		Title:        req.Title,
		Provider:     req.Provider,
		Model:        req.Model,
		SystemPrompt: systemPrompt,
		Settings:     req.Settings,
	}

	if conv.Title == "" {
		conv.Title = "New Conversation"
	}

	if err := h.storage.CreateConversation(conv); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(conv)
}

func (h *Handler) GetConversation(c *fiber.Ctx) error {
	id := c.Params("id")

	conv, err := h.storage.GetConversation(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if conv == nil {
		return c.Status(404).JSON(fiber.Map{"error": "conversation not found"})
	}

	return c.JSON(conv)
}

func (h *Handler) UpdateConversation(c *fiber.Ctx) error {
	id := c.Params("id")

	conv, err := h.storage.GetConversation(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if conv == nil {
		return c.Status(404).JSON(fiber.Map{"error": "conversation not found"})
	}

	var update models.UpdateConversationRequest
	if err := c.BodyParser(&update); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if update.Title != nil {
		conv.Title = *update.Title
	}
	if update.Model != nil {
		conv.Model = *update.Model
	}
	if update.SystemPrompt != nil {
		conv.SystemPrompt = *update.SystemPrompt
	}
	if update.Settings != nil {
		conv.Settings = update.Settings
	}

	if err := h.storage.UpdateConversation(conv); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(conv)
}

func (h *Handler) DeleteConversation(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := h.storage.DeleteConversation(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(204)
}

func (h *Handler) ExportConversation(c *fiber.Ctx) error {
	id := c.Params("id")
	format := c.Query("format", "json")

	conv, err := h.storage.GetConversation(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if conv == nil {
		return c.Status(404).JSON(fiber.Map{"error": "conversation not found"})
	}

	messages, err := h.storage.GetConversationMessages(id, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	switch format {
	case "markdown":
		md := fmt.Sprintf("# %s\n\n", conv.Title)
		md += fmt.Sprintf("Provider: %s | Model: %s\n\n", conv.Provider, conv.Model)
		md += "---\n\n"

		for _, msg := range messages {
			// Capitalize role
			role := msg.Role
			if len(role) > 0 {
				role = strings.ToUpper(role[:1]) + role[1:]
			}
			md += fmt.Sprintf("## %s\n\n%s\n\n", role, msg.Content)
		}

		c.Set("Content-Type", "text/markdown")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.md\"", conv.Title))
		return c.SendString(md)

	default:
		export := fiber.Map{
			"conversation": conv,
			"messages":     messages,
			"exported_at":  time.Now(),
		}
		return c.JSON(export)
	}
}

// Messages
func (h *Handler) GetMessages(c *fiber.Ctx) error {
	convID := c.Params("id")
	parentID := c.Query("parent_id")

	var parent *string
	if parentID != "" {
		parent = &parentID
	}

	messages, err := h.storage.GetConversationMessages(convID, parent)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(messages)
}

// ToolCall represents a pending tool call from the model
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]interface{}
}

func (h *Handler) SendMessage(c *fiber.Ctx) error {
	convID := c.Params("id")

	conv, err := h.storage.GetConversation(convID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if conv == nil {
		return c.Status(404).JSON(fiber.Map{"error": "conversation not found"})
	}

	var req models.SendMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	// Get provider
	prov, ok := h.providers.Get(conv.Provider)
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "provider not found"})
	}

	// Create user message
	userMsg := &models.Message{
		ConversationID: convID,
		Role:           "user",
		Content:        req.Content,
		ParentID:       req.ParentID,
	}

	// Handle attachments
	for _, attID := range req.Attachments {
		att, err := h.storage.GetAttachment(attID)
		if err == nil && att != nil {
			userMsg.Attachments = append(userMsg.Attachments, *att)
		}
	}

	if err := h.storage.CreateMessage(userMsg); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Get conversation history
	messages, err := h.storage.GetConversationMessages(convID, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Create context with cancellation for this stream
	// This allows users to stop generation via StopGeneration endpoint
	ctx, cancel := context.WithCancel(context.Background())
	streamID := uuid.New().String()
	h.activeStreamsMu.Lock()
	h.activeStreams[streamID] = cancel
	h.activeStreamsMu.Unlock()

	// Set up SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Stream-ID", streamID)
	c.Set("X-Accel-Buffering", "no")

	// Create assistant message placeholder
	assistantMsg := &models.Message{
		ConversationID: convID,
		Role:           "assistant",
		Content:        "",
	}

	// Get MCP tools
	tools := h.mcp.GetAllTools()

	// Use streaming response
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() {
			h.activeStreamsMu.Lock()
			delete(h.activeStreams, streamID)
			h.activeStreamsMu.Unlock()
			cancel()
		}()

		// Helper to write SSE event
		writeEvent := func(eventType string, data interface{}) {
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, jsonData)
			w.Flush()
		}

		// Send user message event first
		writeEvent("user_message", fiber.Map{
			"type":            "user_message",
			"id":              userMsg.ID,
			"conversation_id": userMsg.ConversationID,
			"role":            userMsg.Role,
			"content":         userMsg.Content,
			"created_at":      userMsg.CreatedAt,
			"attachments":     userMsg.Attachments,
		})

		// Build chat options from conversation settings
		var chatOpts *provider.ChatOptions
		if conv.Settings != nil {
			thinkingBudget := ""
			if conv.Settings.ThinkingBudget != nil {
				thinkingBudget = *conv.Settings.ThinkingBudget
			}
			chatOpts = &provider.ChatOptions{
				EnableThinking: conv.Settings.EnableThinking != nil && *conv.Settings.EnableThinking,
				EnableTools:    conv.Settings.EnableTools != nil && *conv.Settings.EnableTools,
				Temperature:    conv.Settings.Temperature,
				MaxTokens:      conv.Settings.MaxTokens,
				TopP:           conv.Settings.TopP,
				ThinkingBudget: thinkingBudget,
			}
		}

		// Tool calling loop - configurable max iterations to prevent infinite loops
		maxToolIterations := 10
		if conv.Settings != nil && conv.Settings.MaxToolIterations != nil {
			maxToolIterations = *conv.Settings.MaxToolIterations
			if maxToolIterations < 1 {
				maxToolIterations = 1
			} else if maxToolIterations > 50 {
				maxToolIterations = 50 // Hard limit for safety
			}
		}

		// Apply context management based on conversation settings
		currentMessages := messages
		if conv.Settings != nil {
			contextMode := "manual" // default
			if conv.Settings.ContextMode != nil {
				contextMode = *conv.Settings.ContextMode
			} else if conv.Settings.MaxHistoryLength != nil {
				// Backwards compatibility: if maxHistoryLength is set but no mode, use sliding_window
				contextMode = "sliding_window"
			}

			switch contextMode {
			case "sliding_window":
				// Keep only last N messages
				maxHistory := 50 // default
				if conv.Settings.MaxHistoryLength != nil {
					maxHistory = *conv.Settings.MaxHistoryLength
				}
				if len(currentMessages) > maxHistory {
					currentMessages = currentMessages[len(currentMessages)-maxHistory:]
					log.Printf("Sliding window applied for conversation %s: %d -> %d messages",
						convID, len(messages), len(currentMessages))
				}

			case "auto_compact":
				// Use the context manager for intelligent processing
				threshold := 30 // default threshold
				if conv.Settings.AutoCompactThreshold != nil {
					threshold = *conv.Settings.AutoCompactThreshold
				}
				maxTokens := 80000 // default token budget
				if conv.Settings.MaxContextTokens != nil {
					maxTokens = *conv.Settings.MaxContextTokens
				}

				if len(currentMessages) > threshold {
					// Create context manager with appropriate config
					mgr := ctxmgr.NewManager(config.ContextConfig{
						MaxMessages:      threshold,
						MaxTokens:        maxTokens,
						TruncateLongMsgs: true,
						MaxMsgLength:     4000,
					}, nil)

					// Process the context
					processed, err := mgr.ProcessContext(currentMessages, conv.SystemPrompt, nil)
					if err == nil && len(processed.Messages) > 0 {
						currentMessages = processed.Messages
						// Log that context was optimized
						if processed.WasTruncated || processed.WasSummarized {
							log.Printf("Context optimized for conversation %s: %d -> %d messages (truncated: %v, summarized: %v)",
								convID, len(messages), len(currentMessages), processed.WasTruncated, processed.WasSummarized)
						}
					}
				}

			// case "manual": no-op, use full message list
			}
		}

		var allToolCalls []models.ToolCallInfo // Accumulate all tool calls across iterations

		for iteration := 0; iteration < maxToolIterations; iteration++ {
			var fullContent strings.Builder
			var thinkingContent strings.Builder
			var lastMetrics *models.Metrics
			var debugData interface{}
			var pendingToolCalls []ToolCall
			isFirstIteration := iteration == 0

			// Send iteration start event
			writeEvent("iteration_start", fiber.Map{
				"type":           "iteration_start",
				"iteration":      iteration + 1,
				"max_iterations": maxToolIterations,
			})

			callback := func(event models.StreamEvent) {
				switch event.Type {
				case "debug":
					debugData = event.Data
					writeEvent("debug", event)

				case "start":
					if isFirstIteration {
						writeEvent("start", fiber.Map{"type": "start", "message_id": assistantMsg.ID})
					}

				case "thinking":
					thinkingContent.WriteString(event.Content)
					writeEvent("thinking", fiber.Map{
						"type":    "thinking",
						"content": event.Content,
					})

				case "delta":
					if thinkingContent.Len() > 0 && fullContent.Len() == 0 {
						fullContent.WriteString("<think>")
						fullContent.WriteString(thinkingContent.String())
						fullContent.WriteString("</think>\n\n")
					}
					fullContent.WriteString(event.Content)
					writeEvent("delta", event)

				case "tool_start":
					// Tool call started - create placeholder
					if data, ok := event.Data.(map[string]interface{}); ok {
						tc := ToolCall{
							ID:   fmt.Sprintf("%v", data["id"]),
							Name: fmt.Sprintf("%v", data["name"]),
						}
						pendingToolCalls = append(pendingToolCalls, tc)
					}
					// Add iteration info to tool_start event
					writeEvent("tool_start", fiber.Map{
						"type":      "tool_start",
						"data":      event.Data,
						"iteration": iteration + 1,
					})

				case "tool_delta":
					writeEvent("tool_delta", event)

				case "tool_complete":
					// Tool call complete with arguments - update the pending tool call
					if data, ok := event.Data.(map[string]interface{}); ok {
						toolID := fmt.Sprintf("%v", data["id"])
						for i := range pendingToolCalls {
							if pendingToolCalls[i].ID == toolID {
								if args, ok := data["arguments"].(map[string]interface{}); ok {
									pendingToolCalls[i].Arguments = args
								}
								break
							}
						}
					}
					writeEvent("tool_complete", event)

				case "metrics":
					lastMetrics = event.Metrics
					writeEvent("metrics", event)

				case "error":
					writeEvent("error", event)
				}
			}

			// Call provider
			var chatErr error
			if len(tools) > 0 {
				chatErr = prov.ChatWithTools(ctx, currentMessages, conv.Model, conv.SystemPrompt, tools, chatOpts, callback)
			} else {
				chatErr = prov.Chat(ctx, currentMessages, conv.Model, conv.SystemPrompt, chatOpts, callback)
			}

			if chatErr != nil && ctx.Err() == nil {
				log.Printf("Chat error: %v", chatErr)
				writeEvent("error", fiber.Map{"type": "error", "error": chatErr.Error()})
				break
			}

			// Check if context was cancelled
			if ctx.Err() != nil {
				break
			}

			// If no tool calls, we're done
			if len(pendingToolCalls) == 0 {
				// Handle thinking-only content
				if fullContent.Len() == 0 && thinkingContent.Len() > 0 {
					fullContent.WriteString("<think>")
					fullContent.WriteString(thinkingContent.String())
					fullContent.WriteString("</think>")
				}

				// Save assistant message with accumulated tool calls
				assistantMsg.Content = fullContent.String()
				assistantMsg.Metrics = lastMetrics
				assistantMsg.ToolCalls = allToolCalls // Include all tool calls from all iterations
				h.storage.CreateMessage(assistantMsg)

				// Update conversation title if first message
				if len(messages) <= 1 {
					title := req.Content
					if len(title) > 50 {
						title = title[:50] + "..."
					}
					conv.Title = title
					h.storage.UpdateConversation(conv)
				}

				writeEvent("done", fiber.Map{
					"type":            "done",
					"message_id":      assistantMsg.ID,
					"debug":           debugData,
					"total_iterations": iteration + 1,
				})
				break
			}

			// Execute tool calls
			log.Printf("Executing %d tool calls", len(pendingToolCalls))

			// Build assistant message content with tool calls info
			var assistantContent strings.Builder
			if fullContent.Len() > 0 {
				assistantContent.WriteString(fullContent.String())
			}

			// Collect all tool calls and results
			var toolCalls []models.ToolCallInfo
			var toolResults []models.ToolResultInfo

			// Add tool call messages to conversation
			for _, tc := range pendingToolCalls {
				writeEvent("tool_executing", fiber.Map{
					"type":      "tool_executing",
					"id":        tc.ID,
					"name":      tc.Name,
					"iteration": iteration + 1,
				})

				// Execute tool via MCP
				result, err := h.mcp.CallTool(ctx, tc.Name, tc.Arguments)

				var toolResultContent string
				var isError bool
				if err != nil {
					toolResultContent = fmt.Sprintf("Error: %v", err)
					isError = true
					log.Printf("Tool %s error: %v", tc.Name, err)
				} else {
					toolResultContent = result
					log.Printf("Tool %s result: %s", tc.Name, truncateString(result, 100))
				}

				writeEvent("tool_result", fiber.Map{
					"type":      "tool_result",
					"id":        tc.ID,
					"name":      tc.Name,
					"content":   truncateString(toolResultContent, 500),
					"is_error":  isError,
					"iteration": iteration + 1,
				})

				// Collect tool call and result with proper types
				toolCallInfo := models.ToolCallInfo{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Arguments,
				}
				toolCalls = append(toolCalls, toolCallInfo)

				// Also accumulate for persistence
				allToolCalls = append(allToolCalls, models.ToolCallInfo{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Arguments,
					Result:    toolResultContent,
					IsError:   isError,
				})

				toolResults = append(toolResults, models.ToolResultInfo{
					ToolUseID: tc.ID,
					Content:   toolResultContent,
					IsError:   isError,
				})
			}

			// Add assistant message with tool calls
			toolCallMsg := models.Message{
				Role:      "assistant",
				Content:   fullContent.String(),
				ToolCalls: toolCalls,
			}
			// Add user message with tool results
			toolResultMsg := models.Message{
				Role:        "user",
				ToolResults: toolResults,
			}
			currentMessages = append(currentMessages, toolCallMsg, toolResultMsg)

			// Send iteration end event before continuing to next iteration
			writeEvent("iteration_end", fiber.Map{
				"type":        "iteration_end",
				"iteration":   iteration + 1,
				"tool_count":  len(pendingToolCalls),
				"has_more":    iteration+1 < maxToolIterations,
			})

			// Continue loop for next model response
		}
	})

	return nil
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (h *Handler) RegenerateMessage(c *fiber.Ctx) error {
	convID := c.Params("id")

	var req models.RegenerateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	// Get the message to regenerate
	msg, err := h.storage.GetMessage(req.MessageID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if msg == nil || msg.Role != "assistant" {
		return c.Status(400).JSON(fiber.Map{"error": "invalid message"})
	}

	// Delete the old message
	if err := h.storage.DeleteMessage(msg.ID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Get conversation
	conv, _ := h.storage.GetConversation(convID)
	if conv == nil {
		return c.Status(404).JSON(fiber.Map{"error": "conversation not found"})
	}

	// Get remaining messages
	messages, _ := h.storage.GetConversationMessages(convID, nil)

	// Get provider
	prov, ok := h.providers.Get(conv.Provider)
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "provider not found"})
	}

	// Create context with cancellation for regeneration stream
	ctx, cancel := context.WithCancel(context.Background())
	streamID := uuid.New().String()
	h.activeStreamsMu.Lock()
	h.activeStreams[streamID] = cancel
	h.activeStreamsMu.Unlock()

	// Set up SSE (Server-Sent Events) headers for real-time streaming
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Stream-ID", streamID)
	c.Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Create new assistant message for the regenerated response
	assistantMsg := &models.Message{
		ConversationID: convID,
		Role:           "assistant",
		Content:        "",
	}

	// Use streaming response
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() {
			h.activeStreamsMu.Lock()
			delete(h.activeStreams, streamID)
			h.activeStreamsMu.Unlock()
			cancel()
		}()

		writeEvent := func(eventType string, data interface{}) {
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, jsonData)
			w.Flush()
		}

		var fullContent strings.Builder
		var lastMetrics *models.Metrics

		callback := func(event models.StreamEvent) {
			switch event.Type {
			case "delta":
				fullContent.WriteString(event.Content)
				writeEvent("delta", event)
			case "metrics":
				lastMetrics = event.Metrics
				writeEvent("metrics", event)
			case "done":
				assistantMsg.Content = fullContent.String()
				assistantMsg.Metrics = lastMetrics
				h.storage.CreateMessage(assistantMsg)
				writeEvent("done", fiber.Map{
					"type":       "done",
					"message_id": assistantMsg.ID,
				})
			default:
				writeEvent(event.Type, event)
			}
		}

		// Build chat options from conversation settings
		var chatOpts *provider.ChatOptions
		if conv.Settings != nil {
			thinkingBudget := ""
			if conv.Settings.ThinkingBudget != nil {
				thinkingBudget = *conv.Settings.ThinkingBudget
			}
			chatOpts = &provider.ChatOptions{
				EnableThinking: conv.Settings.EnableThinking != nil && *conv.Settings.EnableThinking,
				EnableTools:    conv.Settings.EnableTools != nil && *conv.Settings.EnableTools,
				Temperature:    conv.Settings.Temperature,
				MaxTokens:      conv.Settings.MaxTokens,
				TopP:           conv.Settings.TopP,
				ThinkingBudget: thinkingBudget,
			}
		}

		tools := h.mcp.GetAllTools()
		if len(tools) > 0 {
			prov.ChatWithTools(ctx, messages, conv.Model, conv.SystemPrompt, tools, chatOpts, callback)
		} else {
			prov.Chat(ctx, messages, conv.Model, conv.SystemPrompt, chatOpts, callback)
		}
	})

	return nil
}

// StopGeneration cancels an ongoing LLM generation stream.
// Users can use this to interrupt long-running responses.
// The stream_id is provided in the X-Stream-ID header of the original request.
func (h *Handler) StopGeneration(c *fiber.Ctx) error {
	streamID := c.Query("stream_id")
	if streamID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "stream_id required"})
	}

	h.activeStreamsMu.Lock()
	cancel, ok := h.activeStreams[streamID]
	if ok {
		cancel()
		delete(h.activeStreams, streamID)
	}
	h.activeStreamsMu.Unlock()

	if ok {
		return c.JSON(fiber.Map{"status": "stopped"})
	}

	return c.Status(404).JSON(fiber.Map{"error": "stream not found"})
}

// Compare
func (h *Handler) CompareProviders(c *fiber.Ctx) error {
	var req models.CompareRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	// Set up SSE
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	// Create user message
	userMsg := models.Message{
		Role:    "user",
		Content: req.Content,
	}

	// Use sync.WaitGroup for proper synchronization
	var wg sync.WaitGroup
	var mu sync.Mutex // Protect concurrent writes

	// Run comparisons concurrently
	for _, selection := range req.Providers {
		prov, ok := h.providers.Get(selection.Provider)
		if !ok {
			continue
		}

		providerID := selection.Provider
		modelID := selection.Model
		wg.Add(1)

		go func(p provider.Provider, provID, modID string) {
			defer wg.Done()

			callback := func(event models.StreamEvent) {
				data := fiber.Map{
					"provider": provID,
					"model":    modID,
					"event":    event,
				}
				jsonData, _ := json.Marshal(data)

				mu.Lock()
				c.Write([]byte(fmt.Sprintf("data: %s\n\n", jsonData)))
				mu.Unlock()
			}

			p.Chat(c.Context(), []models.Message{userMsg}, modID, "", nil, callback)
		}(prov, providerID, modelID)
	}

	// Wait for all providers to complete
	wg.Wait()

	return nil
}

// Files
func (h *Handler) UploadFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "no file provided"})
	}

	// Generate ID and path
	id := uuid.New().String()
	ext := filepath.Ext(file.Filename)
	filename := id + ext
	uploadPath := filepath.Join("uploads", filename)

	// Ensure uploads directory exists
	if err := os.MkdirAll("uploads", 0755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create uploads directory"})
	}

	// Save file
	if err := c.SaveFile(file, uploadPath); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to save file"})
	}

	// Read file for base64 (for images)
	var data string
	mimeType := file.Header.Get("Content-Type")
	if strings.HasPrefix(mimeType, "image/") {
		f, err := os.Open(uploadPath)
		if err == nil {
			defer f.Close()
			reader := bufio.NewReader(f)
			content, _ := io.ReadAll(reader)
			data = base64.StdEncoding.EncodeToString(content)
		}
	}

	att := &models.Attachment{
		ID:       id,
		Filename: file.Filename,
		MimeType: mimeType,
		Size:     file.Size,
		Path:     uploadPath,
		Data:     data,
	}

	// Note: We don't save to DB yet - will be done when message is created
	return c.JSON(att)
}

func (h *Handler) GetAttachment(c *fiber.Ctx) error {
	id := c.Params("id")

	att, err := h.storage.GetAttachment(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if att == nil {
		return c.Status(404).JSON(fiber.Map{"error": "attachment not found"})
	}

	return c.SendFile(att.Path)
}

// MCP
func (h *Handler) ListMCPTools(c *fiber.Ctx) error {
	tools := h.mcp.GetAllTools()
	return c.JSON(tools)
}

func (h *Handler) GetMCPStatus(c *fiber.Ctx) error {
	status := h.mcp.GetStatus()
	return c.JSON(status)
}

// Context Stats
func (h *Handler) GetContextStats(c *fiber.Ctx) error {
	convID := c.Params("id")

	conv, err := h.storage.GetConversation(convID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if conv == nil {
		return c.Status(404).JSON(fiber.Map{"error": "conversation not found"})
	}

	messages, err := h.storage.GetConversationMessages(convID, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Calculate stats manually (inline version of context manager logic)
	estimateTokens := func(text string) int {
		return len(text) / 4
	}

	totalTokens := estimateTokens(conv.SystemPrompt)
	for _, msg := range messages {
		totalTokens += estimateTokens(msg.Content) + 10
		for range msg.Attachments {
			totalTokens += 1000 // Rough estimate for attachments
		}
	}

	maxTokens := h.config.Context.MaxTokens
	if maxTokens == 0 {
		maxTokens = 100000
	}

	percentUsed := float64(totalTokens) / float64(maxTokens) * 100
	needsOpt := percentUsed > 70 || (h.config.Context.MaxMessages > 0 && len(messages) > h.config.Context.MaxMessages*80/100)

	action := ""
	if percentUsed > 90 {
		action = "critical"
	} else if percentUsed > 70 {
		action = "warning"
	} else if percentUsed > 50 {
		action = "info"
	} else {
		action = "ok"
	}

	// Calculate cost estimate using model-specific pricing
	pricing := provider.GetModelPricing(conv.Provider, conv.Model)
	estimatedInputCost := provider.CalculateInputCost(conv.Provider, conv.Model, totalTokens)

	return c.JSON(fiber.Map{
		"message_count":        len(messages),
		"estimated_tokens":     totalTokens,
		"max_tokens":           maxTokens,
		"token_percent_used":   percentUsed,
		"needs_optimization":   needsOpt,
		"status":               action,
		"max_messages":         h.config.Context.MaxMessages,
		"estimated_input_cost": estimatedInputCost,
		"input_price_per_1m":   pricing.InputPer1M,
		"output_price_per_1m":  pricing.OutputPer1M,
		"is_local_provider":    provider.IsLocalProvider(conv.Provider),
		"caching_enabled":      conv.Provider == "claude",
		"recommendations":      getRecommendations(percentUsed, len(messages), h.config.Context.MaxMessages, conv.Provider),
	})
}

func getRecommendations(percentUsed float64, msgCount, maxMessages int, providerName string) []string {
	recs := []string{}

	if percentUsed > 90 {
		recs = append(recs, "Start a new conversation to reduce costs")
		recs = append(recs, "Consider exporting this conversation first")
	} else if percentUsed > 70 {
		recs = append(recs, "Approaching context limit - consider new conversation soon")
	}

	if maxMessages > 0 && msgCount > maxMessages*80/100 {
		recs = append(recs, "Message count high - older messages may be summarized")
	}

	// Only show caching message for Claude (which supports prompt caching)
	if percentUsed > 50 && providerName == "claude" {
		recs = append(recs, "Prompt caching is reducing your costs by up to 90%")
	}

	// Show local inference benefit for Ollama
	if provider.IsLocalProvider(providerName) {
		recs = append(recs, "Running locally - cost is electricity only (~$0.01-0.25/1M tokens)")
	}

	return recs
}

// GetContextBreakdown returns token breakdown per message
func (h *Handler) GetContextBreakdown(c *fiber.Ctx) error {
	convID := c.Params("id")

	conv, err := h.storage.GetConversation(convID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if conv == nil {
		return c.Status(404).JSON(fiber.Map{"error": "conversation not found"})
	}

	messages, err := h.storage.GetConversationMessages(convID, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	estimateTokens := func(text string) int {
		return len(text) / 4
	}

	type MessageBreakdown struct {
		ID              string  `json:"id"`
		Role            string  `json:"role"`
		Tokens          int     `json:"tokens"`
		Percent         float64 `json:"percent"`
		ContentPreview  string  `json:"content_preview"`
		HasAttachments  bool    `json:"has_attachments"`
		AttachmentCount int     `json:"attachment_count"`
		CreatedAt       string  `json:"created_at"`
	}

	// Calculate total
	systemTokens := estimateTokens(conv.SystemPrompt)
	totalTokens := systemTokens

	for _, msg := range messages {
		msgTokens := estimateTokens(msg.Content) + 10
		for _, att := range msg.Attachments {
			if strings.HasPrefix(att.MimeType, "image/") {
				msgTokens += 1000
			} else {
				msgTokens += 100
			}
		}
		totalTokens += msgTokens
	}

	breakdown := make([]MessageBreakdown, 0, len(messages)+1)

	// Add system prompt as first item
	if conv.SystemPrompt != "" {
		preview := conv.SystemPrompt
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		breakdown = append(breakdown, MessageBreakdown{
			ID:             "system",
			Role:           "system",
			Tokens:         systemTokens,
			Percent:        float64(systemTokens) / float64(totalTokens) * 100,
			ContentPreview: preview,
		})
	}

	// Add messages
	for _, msg := range messages {
		msgTokens := estimateTokens(msg.Content) + 10
		for _, att := range msg.Attachments {
			if strings.HasPrefix(att.MimeType, "image/") {
				msgTokens += 1000
			} else {
				msgTokens += 100
			}
		}

		preview := msg.Content
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}

		breakdown = append(breakdown, MessageBreakdown{
			ID:              msg.ID,
			Role:            msg.Role,
			Tokens:          msgTokens,
			Percent:         float64(msgTokens) / float64(totalTokens) * 100,
			ContentPreview:  preview,
			HasAttachments:  len(msg.Attachments) > 0,
			AttachmentCount: len(msg.Attachments),
			CreatedAt:       msg.CreatedAt.Format(time.RFC3339),
		})
	}

	return c.JSON(fiber.Map{
		"total_tokens":  totalTokens,
		"system_tokens": systemTokens,
		"message_count": len(messages),
		"breakdown":     breakdown,
	})
}

// CompactContext performs manual context compaction
func (h *Handler) CompactContext(c *fiber.Ctx) error {
	convID := c.Params("id")

	var req struct {
		Strategy     string `json:"strategy"`      // "summarize", "drop_oldest", "smart"
		TargetTokens int    `json:"target_tokens"` // Target token count
		KeepRecent   int    `json:"keep_recent"`   // Number of recent messages to keep
		PreviewOnly  bool   `json:"preview_only"`  // If true, only show preview without applying
	}
	if err := c.BodyParser(&req); err != nil {
		req.Strategy = "summarize"
		req.KeepRecent = 5
	}

	conv, err := h.storage.GetConversation(convID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if conv == nil {
		return c.Status(404).JSON(fiber.Map{"error": "conversation not found"})
	}

	messages, err := h.storage.GetConversationMessages(convID, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	estimateTokens := func(text string) int {
		return len(text) / 4
	}

	// Calculate original stats
	originalTokens := estimateTokens(conv.SystemPrompt)
	for _, msg := range messages {
		originalTokens += estimateTokens(msg.Content) + 10
	}

	if len(messages) <= req.KeepRecent {
		return c.JSON(fiber.Map{
			"status":           "no_change",
			"message":          "Not enough messages to compact",
			"original_tokens":  originalTokens,
			"new_tokens":       originalTokens,
			"messages_removed": 0,
		})
	}

	// Messages to summarize/remove
	toProcess := messages[:len(messages)-req.KeepRecent]
	toKeep := messages[len(messages)-req.KeepRecent:]

	var summary string
	var removedCount int

	switch req.Strategy {
	case "drop_oldest":
		// Simply drop old messages
		removedCount = len(toProcess)
		summary = ""

	case "smart":
		// Keep important messages (questions, key answers)
		var important []models.Message
		for _, msg := range toProcess {
			// Keep messages with questions or key indicators
			content := strings.ToLower(msg.Content)
			if strings.Contains(content, "?") ||
				strings.Contains(content, "important") ||
				strings.Contains(content, "key") ||
				strings.Contains(content, "summary") ||
				len(msg.Attachments) > 0 {
				important = append(important, msg)
			}
		}
		toProcess = important
		summary = generateSummaryText(toProcess)
		removedCount = len(messages) - req.KeepRecent - len(important)

	default: // "summarize"
		summary = generateSummaryText(toProcess)
		removedCount = len(toProcess)
	}

	// Calculate new token count
	newTokens := estimateTokens(conv.SystemPrompt)
	if summary != "" {
		newTokens += estimateTokens(summary) + 20 // Summary overhead
	}
	for _, msg := range toKeep {
		newTokens += estimateTokens(msg.Content) + 10
	}

	result := fiber.Map{
		"status":           "preview",
		"original_tokens":  originalTokens,
		"new_tokens":       newTokens,
		"tokens_saved":     originalTokens - newTokens,
		"percent_saved":    float64(originalTokens-newTokens) / float64(originalTokens) * 100,
		"messages_removed": removedCount,
		"messages_kept":    len(toKeep),
		"summary":          summary,
		"strategy":         req.Strategy,
	}

	// If not preview only, actually apply the compaction
	if !req.PreviewOnly && removedCount > 0 {
		// Delete old messages
		for _, msg := range messages[:len(messages)-req.KeepRecent] {
			h.storage.DeleteMessage(msg.ID)
		}

		// If we have a summary, create a system message with it
		if summary != "" {
			summaryMsg := &models.Message{
				ConversationID: convID,
				Role:           "system",
				Content:        fmt.Sprintf("[Shrnutí předchozí konverzace: %s]", summary),
			}
			h.storage.CreateMessage(summaryMsg)
		}

		result["status"] = "applied"
		result["message"] = fmt.Sprintf("Kompaktováno: odstraněno %d zpráv, ušetřeno %d tokenů", removedCount, originalTokens-newTokens)
	}

	return c.JSON(result)
}

// generateSummaryText creates a text summary of messages
func generateSummaryText(messages []models.Message) string {
	if len(messages) == 0 {
		return ""
	}

	var topics []string
	for _, msg := range messages {
		content := msg.Content
		// Extract first sentence
		if idx := strings.Index(content, ". "); idx > 0 && idx < 150 {
			content = content[:idx+1]
		} else if len(content) > 100 {
			content = content[:100] + "..."
		}

		if msg.Role == "user" {
			topics = append(topics, fmt.Sprintf("Uživatel: %s", content))
		} else if msg.Role == "assistant" {
			topics = append(topics, fmt.Sprintf("Asistent: %s", content))
		}
	}

	// Keep first 3 and last 3 topics if too many
	if len(topics) > 6 {
		topics = append(topics[:3], topics[len(topics)-3:]...)
	}

	return strings.Join(topics, " | ")
}

// GetContextPreview shows what would be sent to the API
func (h *Handler) GetContextPreview(c *fiber.Ctx) error {
	convID := c.Params("id")

	conv, err := h.storage.GetConversation(convID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if conv == nil {
		return c.Status(404).JSON(fiber.Map{"error": "conversation not found"})
	}

	messages, err := h.storage.GetConversationMessages(convID, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Apply max_history_length if set
	maxHistory := 0
	if conv.Settings != nil && conv.Settings.MaxHistoryLength != nil {
		maxHistory = *conv.Settings.MaxHistoryLength
	}
	if maxHistory == 0 && h.config.Context.MaxMessages > 0 {
		maxHistory = h.config.Context.MaxMessages
	}

	previewMessages := messages
	truncated := false
	if maxHistory > 0 && len(messages) > maxHistory {
		previewMessages = messages[len(messages)-maxHistory:]
		truncated = true
	}

	estimateTokens := func(text string) int {
		return len(text) / 4
	}

	type PreviewMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
		Tokens  int    `json:"tokens"`
	}

	preview := make([]PreviewMessage, 0, len(previewMessages)+1)

	// Add system prompt
	if conv.SystemPrompt != "" {
		preview = append(preview, PreviewMessage{
			Role:    "system",
			Content: conv.SystemPrompt,
			Tokens:  estimateTokens(conv.SystemPrompt),
		})
	}

	totalTokens := estimateTokens(conv.SystemPrompt)
	for _, msg := range previewMessages {
		tokens := estimateTokens(msg.Content) + 10
		totalTokens += tokens
		preview = append(preview, PreviewMessage{
			Role:    msg.Role,
			Content: msg.Content,
			Tokens:  tokens,
		})
	}

	return c.JSON(fiber.Map{
		"messages":           preview,
		"total_tokens":       totalTokens,
		"message_count":      len(preview),
		"was_truncated":      truncated,
		"original_count":     len(messages),
		"max_history_length": maxHistory,
	})
}

// Configuration endpoints

// ConfigResponse is the public config sent to frontend (without sensitive paths)
type ConfigResponse struct {
	Providers map[string]ProviderConfigResponse `json:"providers"`
	Prompts   map[string]config.PromptConfig    `json:"prompts"`
	MCP       config.MCPConfig                  `json:"mcp"`
	Context   config.ContextConfig              `json:"context"`
}

type ProviderConfigResponse struct {
	Type    string `json:"type"`
	APIKey  string `json:"api_key"` // Masked for display
	BaseURL string `json:"base_url,omitempty"`
	HasKey  bool   `json:"has_key"` // Whether API key is set
}

func (h *Handler) GetConfig(c *fiber.Ctx) error {
	h.configMu.RLock()
	defer h.configMu.RUnlock()

	resp := ConfigResponse{
		Providers: make(map[string]ProviderConfigResponse),
		Prompts:   h.config.Prompts,
		MCP:       h.config.MCP,
		Context:   h.config.Context,
	}

	for name, prov := range h.config.Providers {
		maskedKey := ""
		hasKey := prov.APIKey != ""
		if hasKey && len(prov.APIKey) > 8 {
			maskedKey = prov.APIKey[:4] + "..." + prov.APIKey[len(prov.APIKey)-4:]
		} else if hasKey {
			maskedKey = "****"
		}

		resp.Providers[name] = ProviderConfigResponse{
			Type:    prov.Type,
			APIKey:  maskedKey,
			BaseURL: prov.BaseURL,
			HasKey:  hasKey,
		}
	}

	return c.JSON(resp)
}

type UpdateConfigRequest struct {
	Providers map[string]ProviderUpdateRequest `json:"providers,omitempty"`
	Prompts   map[string]config.PromptConfig   `json:"prompts,omitempty"`
	MCP       *config.MCPConfig                `json:"mcp,omitempty"`
	Context   *config.ContextConfig            `json:"context,omitempty"`
}

type ProviderUpdateRequest struct {
	APIKey  *string `json:"api_key,omitempty"`
	BaseURL *string `json:"base_url,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

func (h *Handler) UpdateConfig(c *fiber.Ctx) error {
	var req UpdateConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	// Update providers (only credentials, models come from registry)
	for name, update := range req.Providers {
		if prov, ok := h.config.Providers[name]; ok {
			if update.APIKey != nil {
				prov.APIKey = *update.APIKey
			}
			if update.BaseURL != nil {
				prov.BaseURL = *update.BaseURL
			}
			h.config.Providers[name] = prov
		}
	}

	// Update prompts
	if req.Prompts != nil {
		for name, prompt := range req.Prompts {
			h.config.Prompts[name] = prompt
		}
	}

	// Update MCP
	if req.MCP != nil {
		h.config.MCP = *req.MCP
	}

	// Update context settings
	if req.Context != nil {
		h.config.Context = *req.Context
	}

	// Save to file
	savePath := h.configPath
	if savePath == "" {
		// No config file exists - create one in the current directory
		savePath = "config.json"
		h.configPath = savePath
	}

	if err := h.config.Save(savePath); err != nil {
		log.Printf("Failed to save config to %s: %v", savePath, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Nepodařilo se uložit konfiguraci: %v", err),
		})
	}

	log.Printf("Configuration saved to %s", savePath)
	return c.JSON(fiber.Map{
		"status": "ok",
		"path":   savePath,
	})
}

func (h *Handler) GetConfigPath(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"path":      h.configPath,
		"is_set":    h.configPath != "",
		"is_memory": h.configPath == "",
	})
}

// ListOllamaModels fetches available models from Ollama API
func (h *Handler) ListOllamaModels(c *fiber.Ctx) error {
	baseURL := c.Query("base_url", "http://localhost:11434")
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Ollama API endpoint for listing models
	url := baseURL + "/api/tags"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":  "Nelze se připojit k Ollama",
			"detail": err.Error(),
		})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Ollama vrátila chybu",
		})
	}

	var result struct {
		Models []struct {
			Name       string `json:"name"`
			ModifiedAt string `json:"modified_at"`
			Size       int64  `json:"size"`
			Details    struct {
				Family            string `json:"family"`
				ParameterSize     string `json:"parameter_size"`
				QuantizationLevel string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Nelze zpracovat odpověď z Ollama",
		})
	}

	// Known thinking models (models that support the think parameter)
	thinkingModels := map[string]bool{
		"deepseek-r1": true,
		"qwen3":       true,
		"qwq":         true,
		"marco-o1":    true,
	}

	// Build detailed model list
	type ModelInfo struct {
		Name             string `json:"name"`
		Size             int64  `json:"size"`
		Family           string `json:"family,omitempty"`
		ParameterSize    string `json:"parameter_size,omitempty"`
		SupportsThinking bool   `json:"supports_thinking"`
	}

	models := make([]string, len(result.Models))
	modelDetails := make([]ModelInfo, len(result.Models))

	for i, m := range result.Models {
		models[i] = m.Name

		// Check if model supports thinking (by name prefix)
		supportsThinking := false
		nameLower := strings.ToLower(m.Name)
		for prefix := range thinkingModels {
			if strings.HasPrefix(nameLower, prefix) {
				supportsThinking = true
				break
			}
		}

		modelDetails[i] = ModelInfo{
			Name:             m.Name,
			Size:             m.Size,
			Family:           m.Details.Family,
			ParameterSize:    m.Details.ParameterSize,
			SupportsThinking: supportsThinking,
		}
	}

	return c.JSON(fiber.Map{
		"models":        models,
		"model_details": modelDetails,
	})
}

// ListOpenAIModels fetches available models from OpenAI API
func (h *Handler) ListOpenAIModels(c *fiber.Ctx) error {
	// Get API key from config
	openaiCfg, ok := h.config.Providers["openai"]
	if !ok || openaiCfg.APIKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "OpenAI API klíč není nakonfigurován",
		})
	}

	baseURL := openaiCfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	url := strings.TrimSuffix(baseURL, "/") + "/models"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Nelze vytvořit požadavek",
		})
	}
	req.Header.Set("Authorization", "Bearer "+openaiCfg.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":  "Nelze se připojit k OpenAI",
			"detail": err.Error(),
		})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": fmt.Sprintf("OpenAI vrátila chybu: %d", resp.StatusCode),
		})
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Nelze zpracovat odpověď z OpenAI",
		})
	}

	// Filter to only chat models (gpt-*)
	models := []string{}
	for _, m := range result.Data {
		// Include GPT models and o1 models (reasoning)
		if strings.HasPrefix(m.ID, "gpt-") || strings.HasPrefix(m.ID, "o1") {
			models = append(models, m.ID)
		}
	}

	// Sort models
	sort.Strings(models)

	return c.JSON(fiber.Map{
		"models": models,
	})
}

// GetGPUOptions returns available GPU options for Ollama pricing calculation
func (h *Handler) GetGPUOptions(c *fiber.Ctx) error {
	gpuList := provider.GetGPUList()

	type GPUInfo struct {
		ID              string  `json:"id"`
		Name            string  `json:"name"`
		TDP             int     `json:"tdp_watts"`
		VRAM            int     `json:"vram_gb"`
		PromptTokPerSec int     `json:"prompt_tok_per_sec"`
		GenTokPerSec    int     `json:"gen_tok_per_sec"`
		EstInputCost    float64 `json:"est_input_cost_per_1m"`
		EstOutputCost   float64 `json:"est_output_cost_per_1m"`
	}

	gpus := make([]GPUInfo, 0, len(gpuList))
	currentConfig := provider.GetOllamaConfig()

	for id, spec := range gpuList {
		// Calculate estimated costs for this GPU
		testConfig := provider.OllamaConfig{
			GPU:             id,
			ElectricityRate: currentConfig.ElectricityRate,
			PUE:             currentConfig.PUE,
		}
		pricing := provider.CalculateOllamaPricing(testConfig)

		gpus = append(gpus, GPUInfo{
			ID:              id,
			Name:            spec.Name,
			TDP:             spec.TDP,
			VRAM:            spec.VRAM,
			PromptTokPerSec: spec.PromptTokPerSec,
			GenTokPerSec:    spec.GenTokPerSec,
			EstInputCost:    pricing.InputPer1M,
			EstOutputCost:   pricing.OutputPer1M,
		})
	}

	// Sort by name
	sort.Slice(gpus, func(i, j int) bool {
		return gpus[i].Name < gpus[j].Name
	})

	return c.JSON(fiber.Map{
		"gpus": gpus,
	})
}

// GetOllamaConfig returns current Ollama pricing configuration
func (h *Handler) GetOllamaConfig(c *fiber.Ctx) error {
	config := provider.GetOllamaConfig()
	pricing := provider.CalculateOllamaPricing(config)

	gpuSpec, ok := provider.GPUOptions[config.GPU]
	gpuName := config.GPU
	if ok {
		gpuName = gpuSpec.Name
	}

	return c.JSON(fiber.Map{
		"gpu":              config.GPU,
		"gpu_name":         gpuName,
		"electricity_rate": config.ElectricityRate,
		"pue":              config.PUE,
		"calculated_pricing": fiber.Map{
			"input_per_1m":  pricing.InputPer1M,
			"output_per_1m": pricing.OutputPer1M,
		},
	})
}

// UpdateOllamaConfig updates Ollama pricing configuration
func (h *Handler) UpdateOllamaConfig(c *fiber.Ctx) error {
	var req struct {
		GPU             string   `json:"gpu"`
		ElectricityRate *float64 `json:"electricity_rate"`
		PUE             *float64 `json:"pue"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Get current config as base
	config := provider.GetOllamaConfig()

	// Update fields if provided
	if req.GPU != "" {
		if _, ok := provider.GPUOptions[req.GPU]; !ok {
			return c.Status(400).JSON(fiber.Map{"error": "unknown GPU: " + req.GPU})
		}
		config.GPU = req.GPU
	}

	if req.ElectricityRate != nil {
		if *req.ElectricityRate < 0 || *req.ElectricityRate > 1 {
			return c.Status(400).JSON(fiber.Map{"error": "electricity_rate must be between 0 and 1 ($/kWh)"})
		}
		config.ElectricityRate = *req.ElectricityRate
	}

	if req.PUE != nil {
		if *req.PUE < 1.0 || *req.PUE > 3.0 {
			return c.Status(400).JSON(fiber.Map{"error": "pue must be between 1.0 and 3.0"})
		}
		config.PUE = *req.PUE
	}

	// Apply config
	provider.SetOllamaConfig(config)

	// Return updated config with calculated pricing
	pricing := provider.CalculateOllamaPricing(config)

	gpuSpec, ok := provider.GPUOptions[config.GPU]
	gpuName := config.GPU
	if ok {
		gpuName = gpuSpec.Name
	}

	return c.JSON(fiber.Map{
		"gpu":              config.GPU,
		"gpu_name":         gpuName,
		"electricity_rate": config.ElectricityRate,
		"pue":              config.PUE,
		"calculated_pricing": fiber.Map{
			"input_per_1m":  pricing.InputPer1M,
			"output_per_1m": pricing.OutputPer1M,
		},
	})
}

// GetPricing returns pricing information for a provider/model
func (h *Handler) GetPricing(c *fiber.Ctx) error {
	providerName := c.Query("provider")
	modelName := c.Query("model")

	if providerName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "provider query parameter required"})
	}

	pricing := provider.GetModelPricing(providerName, modelName)

	return c.JSON(fiber.Map{
		"provider":      providerName,
		"model":         modelName,
		"input_per_1m":  pricing.InputPer1M,
		"output_per_1m": pricing.OutputPer1M,
		"is_local":      provider.IsLocalProvider(providerName),
	})
}

// ===== llama.cpp handlers =====

// getLlamaCppProvider returns the llama.cpp provider or nil if not configured
func (h *Handler) getLlamaCppProvider() *provider.LlamaCppProvider {
	p, ok := h.providers.Get("llamacpp")
	if !ok {
		return nil
	}
	lcpp, ok := p.(*provider.LlamaCppProvider)
	if !ok {
		return nil
	}
	return lcpp
}

// GetLlamaCppHealth returns health status of llama.cpp server
func (h *Handler) GetLlamaCppHealth(c *fiber.Ctx) error {
	lcpp := h.getLlamaCppProvider()
	if lcpp == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "llama.cpp provider není nakonfigurován",
		})
	}

	ctx := c.Context()
	health, err := lcpp.Health(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":  "Nelze se připojit k llama.cpp serveru",
			"detail": err.Error(),
		})
	}

	return c.JSON(health)
}

// GetLlamaCppProps returns server properties from llama.cpp
func (h *Handler) GetLlamaCppProps(c *fiber.Ctx) error {
	lcpp := h.getLlamaCppProvider()
	if lcpp == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "llama.cpp provider není nakonfigurován",
		})
	}

	ctx := c.Context()
	props, err := lcpp.Props(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":  "Nelze získat vlastnosti llama.cpp serveru",
			"detail": err.Error(),
		})
	}

	return c.JSON(props)
}

// ListLlamaCppModels returns the configured models for llama.cpp
func (h *Handler) ListLlamaCppModels(c *fiber.Ctx) error {
	lcpp := h.getLlamaCppProvider()
	if lcpp == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "llama.cpp provider není nakonfigurován",
		})
	}

	models := lcpp.Models()
	return c.JSON(fiber.Map{
		"models": models,
	})
}

// LlamaCppInfill performs code infill/FIM (Fill-In-Middle)
func (h *Handler) LlamaCppInfill(c *fiber.Ctx) error {
	lcpp := h.getLlamaCppProvider()
	if lcpp == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "llama.cpp provider není nakonfigurován",
		})
	}

	var req struct {
		Prefix     string `json:"prefix"`
		Suffix     string `json:"suffix"`
		InputExtra []struct {
			Filename string `json:"filename"`
			Text     string `json:"text"`
		} `json:"input_extra,omitempty"`
		Temperature *float64 `json:"temperature,omitempty"`
		MaxTokens   *int     `json:"max_tokens,omitempty"`
		TopP        *float64 `json:"top_p,omitempty"`
		TopK        *int     `json:"top_k,omitempty"`
		Grammar     string   `json:"grammar,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Build chat options
	opts := &provider.ChatOptions{}
	if req.Temperature != nil {
		opts.Temperature = req.Temperature
	}
	if req.MaxTokens != nil {
		opts.MaxTokens = req.MaxTokens
	}
	if req.TopP != nil {
		opts.TopP = req.TopP
	}
	if req.TopK != nil {
		opts.TopK = req.TopK
	}
	if req.Grammar != "" {
		opts.Grammar = req.Grammar
	}

	// Build input extra hint
	var hint string
	for _, extra := range req.InputExtra {
		if extra.Text != "" {
			hint += fmt.Sprintf("// %s\n%s\n\n", extra.Filename, extra.Text)
		}
	}

	ctx := c.Context()
	result, err := lcpp.Infill(ctx, req.Prefix, req.Suffix, hint, opts)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Infill selhal",
			"detail": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"content": result,
	})
}

// LlamaCppTokenize tokenizes text into tokens
func (h *Handler) LlamaCppTokenize(c *fiber.Ctx) error {
	lcpp := h.getLlamaCppProvider()
	if lcpp == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "llama.cpp provider není nakonfigurován",
		})
	}

	var req struct {
		Content string `json:"content"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Content == "" {
		return c.Status(400).JSON(fiber.Map{"error": "content is required"})
	}

	ctx := c.Context()
	tokens, err := lcpp.Tokenize(ctx, req.Content)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Tokenizace selhala",
			"detail": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"tokens":      tokens,
		"token_count": len(tokens),
	})
}

// LlamaCppDetokenize converts tokens back to text
func (h *Handler) LlamaCppDetokenize(c *fiber.Ctx) error {
	lcpp := h.getLlamaCppProvider()
	if lcpp == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "llama.cpp provider není nakonfigurován",
		})
	}

	var req struct {
		Tokens []int `json:"tokens"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if len(req.Tokens) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "tokens is required"})
	}

	ctx := c.Context()
	text, err := lcpp.Detokenize(ctx, req.Tokens)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Detokenizace selhala",
			"detail": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"content": text,
	})
}

// LlamaCppEmbedding generates embeddings for text
func (h *Handler) LlamaCppEmbedding(c *fiber.Ctx) error {
	lcpp := h.getLlamaCppProvider()
	if lcpp == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "llama.cpp provider není nakonfigurován",
		})
	}

	var req struct {
		Content string `json:"content"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Content == "" {
		return c.Status(400).JSON(fiber.Map{"error": "content is required"})
	}

	ctx := c.Context()
	embedding, err := lcpp.Embedding(ctx, req.Content)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Generování embedding selhalo",
			"detail": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"embedding":  embedding,
		"dimensions": len(embedding),
	})
}
