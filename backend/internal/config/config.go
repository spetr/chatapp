package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Server    ServerConfig              `json:"server"`
	Database  DatabaseConfig            `json:"database"`
	Providers map[string]ProviderConfig `json:"providers"`
	Prompts   map[string]PromptConfig   `json:"prompts"`
	MCP       MCPConfig                 `json:"mcp"`
	Context   ContextConfig             `json:"context"`
}

type ContextConfig struct {
	MaxMessages      int  `json:"max_messages"`       // Max messages to send (0 = unlimited)
	MaxTokens        int  `json:"max_tokens"`         // Max input tokens (0 = unlimited)
	TruncateLongMsgs bool `json:"truncate_long_msgs"` // Truncate messages over limit
	MaxMsgLength     int  `json:"max_msg_length"`     // Max chars per message when truncating
}

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type DatabaseConfig struct {
	Path string `json:"path"`
}

// ProviderConfig contains only credentials and connection info
// Model lists come from models.Registry
type ProviderConfig struct {
	Type    string `json:"type"`
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

type PromptConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

type MCPConfig struct {
	Servers []MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled bool              `json:"enabled"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set server defaults
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "chatapp.db"
	}

	// Ensure all default providers exist (for credentials)
	ensureDefaultProviders(&cfg)

	return &cfg, nil
}

// ensureDefaultProviders adds any missing providers with their defaults
func ensureDefaultProviders(cfg *Config) {
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}

	defaults := DefaultConfig()
	for name, defaultProv := range defaults.Providers {
		if _, exists := cfg.Providers[name]; !exists {
			cfg.Providers[name] = defaultProv
		}
	}
}

func LoadFromEnvOrDefault() (*Config, error) {
	configPath := FindConfigPath()
	if configPath == "" {
		return DefaultConfig(), nil
	}
	return Load(configPath)
}

// FindConfigPath returns the path to the config file if it exists
func FindConfigPath() string {
	configPath := os.Getenv("CHATAPP_CONFIG")
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	// Try default locations
	candidates := []string{
		"config.json",
		filepath.Join(os.Getenv("HOME"), ".config", "chatapp", "config.json"),
		"/etc/chatapp/config.json",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	return ""
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Database: DatabaseConfig{
			Path: "chatapp.db",
		},
		Providers: map[string]ProviderConfig{
			"claude": {
				Type: "anthropic",
			},
			"openai": {
				Type: "openai",
			},
			"ollama": {
				Type:    "ollama",
				BaseURL: "http://localhost:11434",
			},
			"llamacpp": {
				Type:    "llamacpp",
				BaseURL: "http://localhost:8080",
			},
		},
		Prompts: map[string]PromptConfig{
			"default": {
				Name:        "Default",
				Description: "Default assistant prompt",
				Content:     "You are a helpful assistant.",
			},
			"coder": {
				Name:        "Coder",
				Description: "Programming assistant",
				Content:     "You are an expert programmer. Help with code, explain concepts, and provide working examples.",
			},
			"reasoning": {
				Name:        "Reasoning",
				Description: "Chain of Thought reasoning assistant",
				Content: `You are a thoughtful assistant that thinks step by step.

Before answering any question:
1. First, understand and restate the problem in your own words
2. Break down complex problems into smaller parts
3. Consider different approaches or perspectives
4. Work through your reasoning systematically
5. Synthesize your findings into a clear answer

Show your reasoning process using <think> tags when helpful. This helps users understand how you arrived at your conclusions.`,
			},
			"react": {
				Name:        "ReAct Agent",
				Description: "Reasoning and Acting agent for tool use",
				Content: `You are a ReAct (Reasoning and Acting) agent. You solve problems by iteratively:

1. THINK: Analyze the current situation and what information you need
2. ACT: Use available tools to gather information or perform actions
3. OBSERVE: Examine the results of your actions
4. REPEAT: Continue until you have enough information to answer

When using tools:
- Explain why you're using each tool before calling it
- After receiving results, explain what you learned
- If a tool fails, try alternative approaches
- Synthesize all gathered information into a coherent final answer

Always show your reasoning in <think> tags so users can follow your thought process.`,
			},
			"analyst": {
				Name:        "Analyst",
				Description: "Analytical problem solver with structured thinking",
				Content: `You are an analytical assistant that approaches problems systematically.

For each question or task:
1. CLARIFY: Identify what exactly is being asked
2. CONTEXT: Consider relevant background information
3. ANALYZE: Break down the problem and examine each component
4. EVALUATE: Weigh different options or interpretations
5. CONCLUDE: Provide a well-reasoned answer or recommendation

Use structured formatting (headers, lists, tables) when presenting complex information. Always support conclusions with evidence or reasoning.`,
			},
		},
		MCP: MCPConfig{
			Servers: []MCPServerConfig{},
		},
		Context: ContextConfig{
			MaxMessages:      50,     // Last 50 messages
			MaxTokens:        100000, // 100k input token limit
			TruncateLongMsgs: true,
			MaxMsgLength:     4000, // Truncate msgs over 4k chars
		},
	}
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// HasAPIKey checks if a provider has an API key configured
func (c *Config) HasAPIKey(providerName string) bool {
	if prov, ok := c.Providers[providerName]; ok {
		return prov.APIKey != ""
	}
	return false
}

// GetBaseURL returns the base URL for a provider, with defaults
func (c *Config) GetBaseURL(providerName string) string {
	if prov, ok := c.Providers[providerName]; ok && prov.BaseURL != "" {
		return prov.BaseURL
	}
	// Default URLs
	switch providerName {
	case "ollama":
		return "http://localhost:11434"
	case "llamacpp":
		return "http://localhost:8080"
	}
	return ""
}
