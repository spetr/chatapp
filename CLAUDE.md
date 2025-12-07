# CLAUDE.md - Project Guidelines

This file provides guidance for Claude Code when working on this project.

## Project Overview

ChatApp is a full-featured LLM chat application with:
- **Backend**: Go (Fiber framework) with SQLite storage
- **Frontend**: Vue 3 + TypeScript + PrimeVue + Tailwind CSS
- **Providers**: Anthropic Claude, OpenAI, Ollama, llama.cpp

## Build & Run Commands

```bash
# Development
make dev-backend    # Run Go backend (port 8080)
make dev-frontend   # Run Vue frontend with hot reload (port 5173)

# Production build
make build          # Build both backend and frontend
./bin/chatapp-server

# Testing
make test           # Run all tests
make test-backend   # Go tests only
make test-frontend  # Vue tests only

# Other
make config         # Generate default config.json
make fmt            # Format Go code
make clean          # Remove build artifacts
```

## Project Structure

```
chatapp/
├── backend/
│   ├── cmd/server/main.go      # Entry point
│   └── internal/
│       ├── api/                # HTTP handlers (Fiber)
│       ├── provider/           # LLM provider implementations
│       │   ├── anthropic.go    # Claude API
│       │   ├── openai.go       # OpenAI API
│       │   ├── ollama.go       # Ollama local models
│       │   ├── llamacpp.go     # llama.cpp server
│       │   └── pricing.go      # Token pricing data
│       ├── storage/            # SQLite storage layer
│       ├── mcp/                # MCP client for tools
│       ├── models/             # Data models & registry
│       ├── context/            # Context management
│       └── config/             # Configuration handling
├── frontend/
│   └── src/
│       ├── components/         # Vue components
│       │   ├── MessageBubble.vue
│       │   ├── ChatInput.vue
│       │   └── ...
│       ├── views/              # Page views
│       ├── stores/             # Pinia stores
│       │   └── chat.ts         # Main chat state
│       ├── api/                # API client
│       └── types/              # TypeScript types
├── config.json                 # Configuration (not in git)
├── Makefile
└── docker-compose.yml
```

## Code Style Guidelines

### Go Backend

- Use standard Go formatting (`go fmt`)
- Error handling: Always check and handle errors appropriately
- Logging: Use `log.Printf` for debug info during development
- Provider interface: All providers implement `Provider` interface in `provider/provider.go`
- SSE streaming: Events use `models.StreamEvent` structure

### Vue Frontend

- Use Composition API with `<script setup lang="ts">`
- State management via Pinia stores
- Components use PrimeVue library
- Styling with Tailwind CSS utility classes
- TypeScript strict mode enabled

### Important Patterns

1. **SSE Streaming**: Backend streams events, frontend handles in `chat.ts`:
   - `start`, `delta`, `thinking`, `done`, `error`
   - `tool_start`, `tool_complete`, `tool_executing`, `tool_result`

2. **Vue Reactivity**: When updating arrays, use `.map()` to create new arrays:
   ```typescript
   // Correct - triggers reactivity
   streamingToolCalls.value = streamingToolCalls.value.map((tc, i) => ...)

   // Wrong - may not trigger reactivity
   streamingToolCalls.value[idx] = { ...tc, status: 'completed' }
   ```

3. **Tool IDs**: Must be unique across all providers. Use timestamp-based IDs:
   ```go
   toolID := fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), index)
   ```

4. **Prompt Caching**: Claude provider adds `cache_control` to system prompt and first 80% of messages.

## Configuration

`config.json` contains API keys and is not committed. Generate with:
```bash
make config
```

Required fields:
- `providers.claude.api_key` - Anthropic API key
- `providers.openai.api_key` - OpenAI API key (optional)
- `mcp.servers` - MCP tool servers configuration

## Testing

```bash
# Backend
cd backend && go test ./... -v

# Frontend
cd frontend && npm run test
```

## Common Tasks

### Adding a new provider

1. Create `backend/internal/provider/newprovider.go`
2. Implement `Provider` interface (Chat, ChatWithTools, Name, Models, CountTokens)
3. Register in `cmd/server/main.go`
4. Add to config.json

### Adding new SSE event types

1. Add handler case in `backend/internal/api/handlers.go`
2. Add handling in `frontend/src/stores/chat.ts` handleStreamEvent()

### Modifying UI components

- Message display: `frontend/src/components/MessageBubble.vue`
- Chat input: `frontend/src/components/ChatInput.vue`
- Settings: `frontend/src/components/SettingsPanel.vue`

## Language

- UI text is in Czech (cs-CZ)
- Code comments and documentation in English
