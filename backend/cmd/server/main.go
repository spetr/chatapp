package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/spetr/chatapp/internal/api"
	"github.com/spetr/chatapp/internal/config"
	"github.com/spetr/chatapp/internal/mcp"
	"github.com/spetr/chatapp/internal/models"
	"github.com/spetr/chatapp/internal/provider"
	"github.com/spetr/chatapp/internal/storage"
)

func main() {
	// Parse flags
	configPath := flag.String("config", "", "Path to config file")
	generateConfig := flag.Bool("generate-config", false, "Generate default config file")
	flag.Parse()

	// Generate config if requested
	if *generateConfig {
		cfg := config.DefaultConfig()
		if err := cfg.Save("config.json"); err != nil {
			log.Fatalf("Failed to generate config: %v", err)
		}
		fmt.Println("Generated config.json with default settings")
		fmt.Println("Please edit the file and add your API keys")
		return
	}

	// Load config
	var cfg *config.Config
	var err error
	var actualConfigPath string
	if *configPath != "" {
		cfg, err = config.Load(*configPath)
		actualConfigPath = *configPath
	} else {
		cfg, err = config.LoadFromEnvOrDefault()
		// Try to find the actual config path that was used
		actualConfigPath = config.FindConfigPath()
	}
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize storage
	store, err := storage.NewSQLiteStorage(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer store.Close()

	// Initialize providers
	providers := provider.NewRegistry()
	modelRegistry := models.GetRegistry()

	for name, provCfg := range cfg.Providers {
		// Get models from registry for this provider (use type, not config key name)
		providerModels := modelRegistry.GetModelsForProvider(provCfg.Type)

		switch provCfg.Type {
		case "anthropic":
			if provCfg.APIKey != "" {
				p := provider.NewAnthropicProvider(provCfg.APIKey, providerModels)
				providers.Register(name, p)
				log.Printf("Registered provider: %s with %d models", name, len(providerModels))
			} else {
				log.Printf("Warning: Provider %s has no API key configured", name)
			}
		case "openai":
			if provCfg.APIKey != "" {
				p := provider.NewOpenAIProvider(provCfg.APIKey, providerModels, provCfg.BaseURL)
				providers.Register(name, p)
				log.Printf("Registered provider: %s", name)
			} else {
				log.Printf("Warning: Provider %s has no API key configured", name)
			}
		case "ollama":
			// Ollama doesn't require an API key, models fetched dynamically
			p := provider.NewOllamaProvider(nil, provCfg.BaseURL)
			providers.Register(name, p)
			log.Printf("Registered provider: %s", name)
		case "llamacpp":
			// llama.cpp doesn't require an API key, models fetched dynamically
			p := provider.NewLlamaCppProvider(nil, provCfg.BaseURL)
			providers.Register(name, p)
			log.Printf("Registered provider: %s", name)
		default:
			log.Printf("Unknown provider type: %s", provCfg.Type)
		}
	}

	// Initialize MCP client
	mcpClient := mcp.NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, serverCfg := range cfg.MCP.Servers {
		if err := mcpClient.StartServer(ctx, serverCfg); err != nil {
			log.Printf("Failed to start MCP server %s: %v", serverCfg.Name, err)
		} else {
			log.Printf("Started MCP server: %s", serverCfg.Name)
		}
	}
	defer mcpClient.StopAll()

	// Initialize Fiber
	app := fiber.New(fiber.Config{
		StreamRequestBody: true,
		BodyLimit:         50 * 1024 * 1024, // 50MB
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Serve static files (for standalone mode)
	app.Static("/", "./frontend/dist")

	// API routes
	handler := api.NewHandler(cfg, actualConfigPath, store, providers, mcpClient)
	handler.RegisterRoutes(app)

	// SPA fallback
	app.Get("/*", func(c *fiber.Ctx) error {
		return c.SendFile("./frontend/dist/index.html")
	})

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting server on %s", addr)

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
		app.Shutdown()
	}()

	if err := app.Listen(addr); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
