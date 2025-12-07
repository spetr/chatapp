package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Expected host 0.0.0.0, got %s", cfg.Server.Host)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Database.Path != "chatapp.db" {
		t.Errorf("Expected database path chatapp.db, got %s", cfg.Database.Path)
	}

	if len(cfg.Providers) == 0 {
		t.Error("Expected at least one provider")
	}

	if _, ok := cfg.Providers["claude"]; !ok {
		t.Error("Expected claude provider")
	}

	if _, ok := cfg.Providers["openai"]; !ok {
		t.Error("Expected openai provider")
	}

	if len(cfg.Prompts) == 0 {
		t.Error("Expected at least one prompt template")
	}

	if cfg.Context.MaxMessages != 50 {
		t.Errorf("Expected max messages 50, got %d", cfg.Context.MaxMessages)
	}

	if cfg.Context.MaxTokens != 100000 {
		t.Errorf("Expected max tokens 100000, got %d", cfg.Context.MaxTokens)
	}
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.json")

	configContent := `{
		"server": {
			"host": "127.0.0.1",
			"port": 9090
		},
		"database": {
			"path": "test.db"
		},
		"providers": {
			"test": {
				"type": "anthropic",
				"api_key": "test-key",
				"models": ["model-1"],
				"default": "model-1"
			}
		},
		"prompts": {},
		"context": {
			"max_messages": 100,
			"max_tokens": 50000
		}
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Expected host 127.0.0.1, got %s", cfg.Server.Host)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("Expected port 9090, got %d", cfg.Server.Port)
	}

	if cfg.Database.Path != "test.db" {
		t.Errorf("Expected database path test.db, got %s", cfg.Database.Path)
	}

	if _, ok := cfg.Providers["test"]; !ok {
		t.Error("Expected test provider")
	}

	if cfg.Context.MaxMessages != 100 {
		t.Errorf("Expected max messages 100, got %d", cfg.Context.MaxMessages)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	// Create a minimal config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "minimal_config.json")

	configContent := `{
		"providers": {}
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check that defaults are applied
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Database.Path != "chatapp.db" {
		t.Errorf("Expected default database path chatapp.db, got %s", cfg.Database.Path)
	}
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "save_test.json")

	cfg := DefaultConfig()
	cfg.Server.Port = 3000

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load and verify
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if loaded.Server.Port != 3000 {
		t.Errorf("Expected port 3000, got %d", loaded.Server.Port)
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	if err := os.WriteFile(configPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Expected error loading invalid config")
	}
}

func TestLoadNonexistentConfig(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Error("Expected error loading nonexistent config")
	}
}
