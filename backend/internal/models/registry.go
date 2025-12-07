package models

import (
	"strings"
	"sync"
)

// ModelInfo contains all metadata about a model
type ModelInfo struct {
	ID          string            `json:"id"`           // Full model ID (e.g., "claude-sonnet-4-5-20250929")
	Provider    string            `json:"provider"`     // Provider ID (e.g., "anthropic", "openai")
	DisplayName string            `json:"display_name"` // Human-readable name
	Family      string            `json:"family"`       // Model family (e.g., "sonnet-4.5", "gpt-4o")
	Description string            `json:"description"`  // Short description

	// Pricing per 1M tokens
	Pricing ModelPricing `json:"pricing"`

	// Limits
	ContextWindow int `json:"context_window"` // Max input tokens
	MaxOutput     int `json:"max_output"`     // Max output tokens

	// Capabilities
	Capabilities ModelCapabilities `json:"capabilities"`

	// Metadata
	ReleaseDate  string `json:"release_date,omitempty"`
	IsLatest     bool   `json:"is_latest"`
	IsDeprecated bool   `json:"is_deprecated"`
	IsDefault    bool   `json:"is_default"` // Default model for this provider
}

// ModelPricing contains pricing information
type ModelPricing struct {
	InputPer1M  float64 `json:"input_per_1m"`
	OutputPer1M float64 `json:"output_per_1m"`
}

// ModelCapabilities defines what features a model supports
type ModelCapabilities struct {
	Thinking       bool   `json:"thinking"`                  // Extended thinking/reasoning
	ThinkingBudget bool   `json:"thinking_budget,omitempty"` // Supports budget levels (low/medium/high)
	Tools          bool   `json:"tools"`                     // Function/tool calling
	Vision         bool   `json:"vision"`                    // Image input
	Citations      bool   `json:"citations,omitempty"`       // Document citations (Claude)
	JSON           bool   `json:"json"`                      // JSON output mode
	Streaming      bool   `json:"streaming"`                 // Streaming responses
	PromptCaching  bool   `json:"prompt_caching,omitempty"`  // Prompt caching support
}

// ProviderInfo contains provider metadata
type ProviderInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"` // "cloud" or "local"
	Available   bool   `json:"available"`
	HasAPIKey   bool   `json:"has_api_key"`
}

// ModelRegistry holds all registered models
type ModelRegistry struct {
	mu       sync.RWMutex
	models   map[string]*ModelInfo // key is model ID
	byFamily map[string][]*ModelInfo
}

// Global registry instance
var globalRegistry = NewModelRegistry()

// NewModelRegistry creates a new model registry
func NewModelRegistry() *ModelRegistry {
	r := &ModelRegistry{
		models:   make(map[string]*ModelInfo),
		byFamily: make(map[string][]*ModelInfo),
	}
	r.registerDefaultModels()
	return r
}

// GetRegistry returns the global model registry
func GetRegistry() *ModelRegistry {
	return globalRegistry
}

// Register adds a model to the registry
func (r *ModelRegistry) Register(model *ModelInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.models[model.ID] = model
	r.byFamily[model.Family] = append(r.byFamily[model.Family], model)
}

// Get returns a model by ID
func (r *ModelRegistry) Get(id string) *ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models[id]
}

// GetByProvider returns all models for a provider
func (r *ModelRegistry) GetByProvider(provider string) []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ModelInfo
	for _, m := range r.models {
		if m.Provider == provider {
			result = append(result, m)
		}
	}
	return result
}

// GetDefault returns the default model for a provider
func (r *ModelRegistry) GetDefault(provider string) *ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, m := range r.models {
		if m.Provider == provider && m.IsDefault {
			return m
		}
	}
	return nil
}

// All returns all registered models
func (r *ModelRegistry) All() []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ModelInfo, 0, len(r.models))
	for _, m := range r.models {
		result = append(result, m)
	}
	return result
}

// GetPricing returns pricing for a model, with fallback to family/default
func (r *ModelRegistry) GetPricing(provider, modelID string) ModelPricing {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try exact match
	if m, ok := r.models[modelID]; ok {
		return m.Pricing
	}

	// Try prefix match (for model variants)
	modelLower := strings.ToLower(modelID)
	for id, m := range r.models {
		if strings.HasPrefix(modelLower, strings.ToLower(id)) {
			return m.Pricing
		}
		// Also try matching by family prefix
		if m.Provider == provider && strings.Contains(modelLower, strings.ToLower(m.Family)) {
			return m.Pricing
		}
	}

	// Return default for provider
	if def := r.GetDefault(provider); def != nil {
		return def.Pricing
	}

	// Fallback
	return ModelPricing{InputPer1M: 3.0, OutputPer1M: 15.0}
}

// SupportsThinking checks if a model supports extended thinking
func (r *ModelRegistry) SupportsThinking(modelID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if m, ok := r.models[modelID]; ok {
		return m.Capabilities.Thinking
	}

	// Check by known patterns
	lower := strings.ToLower(modelID)
	thinkingPatterns := []string{"o1", "o3", "o4", "deepseek-r1", "qwen3", "qwq", "marco-o1"}
	for _, pattern := range thinkingPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// registerDefaultModels registers all known models
func (r *ModelRegistry) registerDefaultModels() {
	// ============================================
	// ANTHROPIC CLAUDE MODELS
	// ============================================

	// Claude 4.5 (Latest)
	r.Register(&ModelInfo{
		ID:          "claude-sonnet-4-5-20250929",
		Provider:    "anthropic",
		DisplayName: "Claude Sonnet 4.5",
		Family:      "sonnet-4.5",
		Description: "Smart model for complex agents and coding",
		Pricing:     ModelPricing{InputPer1M: 3.00, OutputPer1M: 15.00},
		ContextWindow: 200000,
		MaxOutput:     64000,
		Capabilities: ModelCapabilities{
			Thinking:       true,
			ThinkingBudget: true,
			Tools:          true,
			Vision:         true,
			Citations:      true,
			JSON:           true,
			Streaming:      true,
			PromptCaching:  true,
		},
		ReleaseDate: "2025-09-29",
		IsLatest:    true,
		IsDefault:   true,
	})

	r.Register(&ModelInfo{
		ID:          "claude-opus-4-5-20251101",
		Provider:    "anthropic",
		DisplayName: "Claude Opus 4.5",
		Family:      "opus-4.5",
		Description: "Premium model with maximum intelligence",
		Pricing:     ModelPricing{InputPer1M: 5.00, OutputPer1M: 25.00},
		ContextWindow: 200000,
		MaxOutput:     64000,
		Capabilities: ModelCapabilities{
			Thinking:       true,
			ThinkingBudget: true,
			Tools:          true,
			Vision:         true,
			Citations:      true,
			JSON:           true,
			Streaming:      true,
			PromptCaching:  true,
		},
		ReleaseDate: "2025-11-01",
		IsLatest:    true,
	})

	r.Register(&ModelInfo{
		ID:          "claude-haiku-4-5-20251001",
		Provider:    "anthropic",
		DisplayName: "Claude Haiku 4.5",
		Family:      "haiku-4.5",
		Description: "Fastest model with near-frontier intelligence",
		Pricing:     ModelPricing{InputPer1M: 1.00, OutputPer1M: 5.00},
		ContextWindow: 200000,
		MaxOutput:     64000,
		Capabilities: ModelCapabilities{
			Thinking:       true,
			ThinkingBudget: true,
			Tools:          true,
			Vision:         true,
			Citations:      true,
			JSON:           true,
			Streaming:      true,
			PromptCaching:  true,
		},
		ReleaseDate: "2025-10-01",
		IsLatest:    true,
	})

	// Claude 4 (Legacy)
	r.Register(&ModelInfo{
		ID:          "claude-sonnet-4-20250514",
		Provider:    "anthropic",
		DisplayName: "Claude Sonnet 4",
		Family:      "sonnet-4",
		Description: "Previous generation Sonnet",
		Pricing:     ModelPricing{InputPer1M: 3.00, OutputPer1M: 15.00},
		ContextWindow: 200000,
		MaxOutput:     64000,
		Capabilities: ModelCapabilities{
			Thinking:       true,
			ThinkingBudget: true,
			Tools:          true,
			Vision:         true,
			Citations:      true,
			JSON:           true,
			Streaming:      true,
			PromptCaching:  true,
		},
		ReleaseDate: "2025-05-14",
	})

	r.Register(&ModelInfo{
		ID:          "claude-opus-4-1-20250805",
		Provider:    "anthropic",
		DisplayName: "Claude Opus 4.1",
		Family:      "opus-4.1",
		Description: "Previous generation Opus",
		Pricing:     ModelPricing{InputPer1M: 15.00, OutputPer1M: 75.00},
		ContextWindow: 200000,
		MaxOutput:     32000,
		Capabilities: ModelCapabilities{
			Thinking:       true,
			ThinkingBudget: true,
			Tools:          true,
			Vision:         true,
			Citations:      true,
			JSON:           true,
			Streaming:      true,
			PromptCaching:  true,
		},
		ReleaseDate: "2025-08-05",
	})

	// Claude 3.5 (Legacy)
	r.Register(&ModelInfo{
		ID:          "claude-3-5-haiku-20241022",
		Provider:    "anthropic",
		DisplayName: "Claude 3.5 Haiku",
		Family:      "haiku-3.5",
		Description: "Fast and affordable legacy model",
		Pricing:     ModelPricing{InputPer1M: 0.80, OutputPer1M: 4.00},
		ContextWindow: 200000,
		MaxOutput:     8000,
		Capabilities: ModelCapabilities{
			Tools:         true,
			Vision:        true,
			JSON:          true,
			Streaming:     true,
			PromptCaching: true,
		},
		ReleaseDate:  "2024-10-22",
		IsDeprecated: true,
	})

	// ============================================
	// OPENAI MODELS
	// ============================================

	// GPT-5 Series (Latest - August 2025)
	r.Register(&ModelInfo{
		ID:          "gpt-5",
		Provider:    "openai",
		DisplayName: "GPT-5",
		Family:      "gpt-5",
		Description: "Most capable GPT model with 400K context",
		Pricing:     ModelPricing{InputPer1M: 1.25, OutputPer1M: 10.00},
		ContextWindow: 400000,
		MaxOutput:     32768,
		Capabilities: ModelCapabilities{
			Tools:     true,
			Vision:    true,
			JSON:      true,
			Streaming: true,
		},
		ReleaseDate: "2025-08-07",
		IsLatest:    true,
		IsDefault:   true,
	})

	r.Register(&ModelInfo{
		ID:          "gpt-5-mini",
		Provider:    "openai",
		DisplayName: "GPT-5 Mini",
		Family:      "gpt-5-mini",
		Description: "Balanced performance and cost",
		Pricing:     ModelPricing{InputPer1M: 0.25, OutputPer1M: 2.00},
		ContextWindow: 400000,
		MaxOutput:     32768,
		Capabilities: ModelCapabilities{
			Tools:     true,
			Vision:    true,
			JSON:      true,
			Streaming: true,
		},
		ReleaseDate: "2025-08-07",
		IsLatest:    true,
	})

	r.Register(&ModelInfo{
		ID:          "gpt-5-nano",
		Provider:    "openai",
		DisplayName: "GPT-5 Nano",
		Family:      "gpt-5-nano",
		Description: "Fastest and cheapest GPT-5 variant",
		Pricing:     ModelPricing{InputPer1M: 0.05, OutputPer1M: 0.40},
		ContextWindow: 400000,
		MaxOutput:     16384,
		Capabilities: ModelCapabilities{
			Tools:     true,
			Vision:    true,
			JSON:      true,
			Streaming: true,
		},
		ReleaseDate: "2025-08-07",
		IsLatest:    true,
	})

	// GPT-4.1 Series (April 2025)
	r.Register(&ModelInfo{
		ID:          "gpt-4.1",
		Provider:    "openai",
		DisplayName: "GPT-4.1",
		Family:      "gpt-4.1",
		Description: "1M context window, improved coding",
		Pricing:     ModelPricing{InputPer1M: 2.00, OutputPer1M: 8.00},
		ContextWindow: 1000000,
		MaxOutput:     32768,
		Capabilities: ModelCapabilities{
			Tools:     true,
			Vision:    true,
			JSON:      true,
			Streaming: true,
		},
		ReleaseDate: "2025-04-14",
		IsLatest:    true,
	})

	r.Register(&ModelInfo{
		ID:          "gpt-4.1-mini",
		Provider:    "openai",
		DisplayName: "GPT-4.1 Mini",
		Family:      "gpt-4.1-mini",
		Description: "Fast and affordable with 1M context",
		Pricing:     ModelPricing{InputPer1M: 0.40, OutputPer1M: 1.60},
		ContextWindow: 1000000,
		MaxOutput:     32768,
		Capabilities: ModelCapabilities{
			Tools:     true,
			Vision:    true,
			JSON:      true,
			Streaming: true,
		},
		ReleaseDate: "2025-04-14",
		IsLatest:    true,
	})

	r.Register(&ModelInfo{
		ID:          "gpt-4.1-nano",
		Provider:    "openai",
		DisplayName: "GPT-4.1 Nano",
		Family:      "gpt-4.1-nano",
		Description: "Ultra-fast, ultra-cheap with 1M context",
		Pricing:     ModelPricing{InputPer1M: 0.10, OutputPer1M: 0.40},
		ContextWindow: 1000000,
		MaxOutput:     16384,
		Capabilities: ModelCapabilities{
			Tools:     true,
			Vision:    true,
			JSON:      true,
			Streaming: true,
		},
		ReleaseDate: "2025-04-14",
		IsLatest:    true,
	})

	// GPT-4o Series (Legacy)
	r.Register(&ModelInfo{
		ID:          "gpt-4o",
		Provider:    "openai",
		DisplayName: "GPT-4o",
		Family:      "gpt-4o",
		Description: "Previous flagship GPT-4 model",
		Pricing:     ModelPricing{InputPer1M: 2.50, OutputPer1M: 10.00},
		ContextWindow: 128000,
		MaxOutput:     16384,
		Capabilities: ModelCapabilities{
			Tools:     true,
			Vision:    true,
			JSON:      true,
			Streaming: true,
		},
	})

	r.Register(&ModelInfo{
		ID:          "gpt-4o-mini",
		Provider:    "openai",
		DisplayName: "GPT-4o Mini",
		Family:      "gpt-4o-mini",
		Description: "Affordable small model for fast tasks",
		Pricing:     ModelPricing{InputPer1M: 0.15, OutputPer1M: 0.60},
		ContextWindow: 128000,
		MaxOutput:     16384,
		Capabilities: ModelCapabilities{
			Tools:     true,
			Vision:    true,
			JSON:      true,
			Streaming: true,
		},
	})

	r.Register(&ModelInfo{
		ID:          "gpt-4-turbo",
		Provider:    "openai",
		DisplayName: "GPT-4 Turbo",
		Family:      "gpt-4-turbo",
		Description: "Previous generation GPT-4",
		Pricing:     ModelPricing{InputPer1M: 10.00, OutputPer1M: 30.00},
		ContextWindow: 128000,
		MaxOutput:     4096,
		Capabilities: ModelCapabilities{
			Tools:     true,
			Vision:    true,
			JSON:      true,
			Streaming: true,
		},
		IsDeprecated: true,
	})

	// OpenAI Reasoning Models (o-series)
	r.Register(&ModelInfo{
		ID:          "o1",
		Provider:    "openai",
		DisplayName: "o1",
		Family:      "o1",
		Description: "Advanced reasoning model",
		Pricing:     ModelPricing{InputPer1M: 15.00, OutputPer1M: 60.00},
		ContextWindow: 200000,
		MaxOutput:     100000,
		Capabilities: ModelCapabilities{
			Thinking:       true,
			ThinkingBudget: true,
			Tools:          true,
			Vision:         true,
			JSON:           true,
			Streaming:      true,
		},
		IsLatest: true,
	})

	r.Register(&ModelInfo{
		ID:          "o1-mini",
		Provider:    "openai",
		DisplayName: "o1 Mini",
		Family:      "o1-mini",
		Description: "Smaller reasoning model",
		Pricing:     ModelPricing{InputPer1M: 3.00, OutputPer1M: 12.00},
		ContextWindow: 128000,
		MaxOutput:     65536,
		Capabilities: ModelCapabilities{
			Thinking:       true,
			ThinkingBudget: true,
			Tools:          true,
			JSON:           true,
			Streaming:      true,
		},
		IsLatest: true,
	})

	r.Register(&ModelInfo{
		ID:          "o3-mini",
		Provider:    "openai",
		DisplayName: "o3 Mini",
		Family:      "o3-mini",
		Description: "Latest compact reasoning model",
		Pricing:     ModelPricing{InputPer1M: 1.10, OutputPer1M: 4.40},
		ContextWindow: 200000,
		MaxOutput:     100000,
		Capabilities: ModelCapabilities{
			Thinking:       true,
			ThinkingBudget: true,
			Tools:          true,
			JSON:           true,
			Streaming:      true,
		},
		IsLatest: true,
	})

	r.Register(&ModelInfo{
		ID:          "o4-mini",
		Provider:    "openai",
		DisplayName: "o4 Mini",
		Family:      "o4-mini",
		Description: "Newest compact reasoning model",
		Pricing:     ModelPricing{InputPer1M: 1.10, OutputPer1M: 4.40},
		ContextWindow: 200000,
		MaxOutput:     100000,
		Capabilities: ModelCapabilities{
			Thinking:       true,
			ThinkingBudget: true,
			Tools:          true,
			Vision:         true,
			JSON:           true,
			Streaming:      true,
		},
		IsLatest: true,
	})
}

// RegisterDynamicModel adds a dynamically discovered model (e.g., from Ollama)
func (r *ModelRegistry) RegisterDynamicModel(provider, modelID, displayName string, capabilities ModelCapabilities) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Don't overwrite existing models
	if _, exists := r.models[modelID]; exists {
		return
	}

	r.models[modelID] = &ModelInfo{
		ID:          modelID,
		Provider:    provider,
		DisplayName: displayName,
		Family:      modelID,
		Pricing:     ModelPricing{InputPer1M: 0, OutputPer1M: 0}, // Free for local
		Capabilities: capabilities,
	}
}

// GetModelsForProvider returns model IDs for a provider (for backward compatibility)
func (r *ModelRegistry) GetModelsForProvider(provider string) []string {
	models := r.GetByProvider(provider)
	result := make([]string, len(models))
	for i, m := range models {
		result[i] = m.ID
	}
	return result
}

// GetDefaultModelID returns the default model ID for a provider
func (r *ModelRegistry) GetDefaultModelID(provider string) string {
	if def := r.GetDefault(provider); def != nil {
		return def.ID
	}
	return ""
}
