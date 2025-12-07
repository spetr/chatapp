# ChatApp - LLM Chat Demo

A full-featured LLM chat application demonstrating streaming, multi-provider support, MCP integration, and cost optimization techniques.

## Features

### Core
- **Multi-provider support** - Claude (Anthropic) and OpenAI with easy extension
- **Real-time streaming** - SSE-based streaming with live Markdown rendering
- **Conversation management** - Create, save, delete, and export conversations
- **File attachments** - Upload images and documents to include in prompts

### Cost Optimization
- **Prompt caching** - Automatic caching for Claude (90% cost reduction on cached tokens)
- **Token tracking** - Real-time monitoring of token usage and costs
- **Context management** - Configurable sliding window to control context size

### Developer Features
- **Debug panel** - View raw API requests, response metrics, and timing
- **Provider comparison** - Compare responses from multiple providers side-by-side
- **MCP support** - Model Context Protocol for tool integration

### UI
- **Dark/Light mode** - System-aware theme switching
- **Responsive design** - Works on desktop and mobile
- **Code highlighting** - Syntax highlighting with copy button

## Quick Start

### Prerequisites
- Go 1.23+
- Node.js 20+
- Docker (optional)

### 1. Generate Config

```bash
make config
# or
cd backend && go run ./cmd/server -generate-config
```

Edit `config.json` and add your API keys:

```json
{
  "providers": {
    "claude": {
      "api_key": "sk-ant-..."
    },
    "openai": {
      "api_key": "sk-..."
    }
  }
}
```

### 2. Run Development

Terminal 1 (Backend):
```bash
make dev-backend
```

Terminal 2 (Frontend):
```bash
make dev-frontend
```

Open http://localhost:5173

### 3. Production Build

```bash
make build
./bin/chatapp-server
```

Or with Docker:
```bash
make docker-build
make docker-up
```

## Configuration

### config.json

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080
  },
  "database": {
    "path": "chatapp.db"
  },
  "providers": {
    "claude": {
      "type": "anthropic",
      "api_key": "YOUR_ANTHROPIC_KEY",
      "models": ["claude-sonnet-4-20250514", "claude-opus-4-20250514"],
      "default": "claude-sonnet-4-20250514"
    },
    "openai": {
      "type": "openai",
      "api_key": "YOUR_OPENAI_KEY",
      "models": ["gpt-4o", "gpt-4o-mini"],
      "default": "gpt-4o"
    }
  },
  "prompts": {
    "default": {
      "name": "Default",
      "description": "General assistant",
      "content": "You are a helpful assistant."
    },
    "coder": {
      "name": "Coder",
      "description": "Programming assistant",
      "content": "You are an expert programmer..."
    }
  },
  "context": {
    "max_messages": 50,
    "max_tokens": 100000,
    "truncate_long_msgs": true,
    "max_msg_length": 4000
  },
  "mcp": {
    "servers": [
      {
        "name": "filesystem",
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"],
        "enabled": true
      }
    ]
  }
}
```

### Environment Variables

- `CHATAPP_CONFIG` - Path to config file (default: `config.json`)

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Vue Frontend  │────▶│   Go Backend    │────▶│  LLM Providers  │
│  PrimeVue + TW  │◀────│   (Fiber)       │◀────│  Claude/OpenAI  │
└─────────────────┘     └────────┬────────┘     └─────────────────┘
                                 │
                        ┌────────▼────────┐
                        │     SQLite      │
                        └─────────────────┘
```

### Project Structure

```
chatapp/
├── backend/
│   ├── cmd/server/         # Main entry point
│   └── internal/
│       ├── api/            # HTTP handlers
│       ├── provider/       # LLM provider implementations
│       ├── storage/        # SQLite storage
│       ├── mcp/            # MCP client
│       ├── models/         # Data models
│       └── config/         # Configuration
├── frontend/
│   ├── src/
│   │   ├── components/     # Vue components
│   │   ├── views/          # Page views
│   │   ├── stores/         # Pinia stores
│   │   ├── api/            # API client
│   │   └── types/          # TypeScript types
│   └── ...
├── config.json             # Configuration file
├── docker-compose.yml
└── Makefile
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/health` | GET | Health check |
| `/api/providers` | GET | List available providers |
| `/api/prompts` | GET | List prompt templates |
| `/api/conversations` | GET | List conversations |
| `/api/conversations` | POST | Create conversation |
| `/api/conversations/:id` | GET | Get conversation |
| `/api/conversations/:id` | DELETE | Delete conversation |
| `/api/conversations/:id/messages` | GET | Get messages |
| `/api/conversations/:id/messages` | POST | Send message (SSE) |
| `/api/conversations/:id/regenerate` | POST | Regenerate last response |
| `/api/conversations/:id/stop` | POST | Stop generation |
| `/api/upload` | POST | Upload file |
| `/api/mcp/tools` | GET | List MCP tools |

## How It Works

### Prompt Caching (Claude)

Claude supports prompt caching which can reduce costs by up to 90% for repeated context:

```
Request 1: [system prompt] + [msg1] + [msg2]  → Full price
Request 2: [system prompt] + [msg1] + [msg2] + [msg3]  → Cached portion = 10% price
```

The backend automatically adds `cache_control: {"type": "ephemeral"}` to:
1. System prompt (always cached)
2. First 80% of conversation history

### Context Management

To prevent token explosion in long conversations:

1. **Token counting** - Estimates tokens before each request
2. **Sliding window** - Configurable max messages to send
3. **Truncation** - Long messages can be automatically shortened
4. **Warning UI** - User sees warning when approaching limits

### Streaming

Uses Server-Sent Events (SSE) for real-time streaming:

```
Client                    Server                    LLM
  │                         │                        │
  ├──POST /messages────────▶│                        │
  │                         ├──────Stream────────────▶
  │◀───event: start─────────┤                        │
  │◀───event: delta─────────┤◀──────chunk───────────│
  │◀───event: delta─────────┤◀──────chunk───────────│
  │◀───event: metrics───────┤◀──────done────────────│
  │◀───event: done──────────┤                        │
```

## Adding a New Provider

1. Create `backend/internal/provider/newprovider.go`:

```go
type NewProvider struct {
    apiKey string
    // ...
}

func (p *NewProvider) Chat(ctx context.Context, messages []models.Message, ...) error {
    // Implement streaming
}
```

2. Register in `cmd/server/main.go`:

```go
case "newprovider":
    p := provider.NewProvider(provCfg.APIKey, provCfg.Models)
    providers.Register(name, p)
```

3. Add to config:

```json
"newprovider": {
    "type": "newprovider",
    "api_key": "...",
    "models": ["model-1", "model-2"]
}
```

## License

MIT
