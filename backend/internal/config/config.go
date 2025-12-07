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
