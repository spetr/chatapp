export interface ConversationSettings {
  // Generation parameters
  temperature?: number        // 0.0-2.0
  max_tokens?: number         // Max response length
  top_p?: number              // 0.0-1.0, nucleus sampling
  top_k?: number              // For Ollama
  frequency_penalty?: number  // -2.0 to 2.0
  presence_penalty?: number   // -2.0 to 2.0
  stop_sequences?: string[]   // Stop generation at these

  // Feature toggles
  stream?: boolean            // Enable streaming (default true)
  enable_thinking?: boolean   // Enable reasoning/thinking mode
  enable_tools?: boolean      // Enable tool/function calling

  // Context management
  context_length?: number     // Custom context window
  max_history_length?: number // Max messages to include

  // Response format
  response_format?: string    // "text" or "json_object"

  // Thinking budget - "low", "medium", "high" for Ollama, or numeric string for Claude
  thinking_budget?: string

  // Ollama-specific
  num_ctx?: number            // Context window size
  num_predict?: number        // Max tokens to predict
  repeat_penalty?: number     // Repetition penalty
  seed?: number               // Random seed for reproducibility
  grammar?: string            // GBNF grammar for structured output
  enable_citations?: boolean  // Enable document citations (Claude)

  // ReAct settings
  max_tool_iterations?: number // Max tool call iterations (default 10, max 50)
}

export interface Conversation {
  id: string
  title: string
  provider: string
  model: string
  system_prompt: string
  settings?: ConversationSettings
  created_at: string
  updated_at: string
}

// Tool call information
export interface ToolCall {
  id: string
  name: string
  arguments: Record<string, unknown>
  result?: string
  error?: string
  status: 'pending' | 'running' | 'completed' | 'error'
  started_at?: string
  completed_at?: string
  iteration?: number  // Which ReAct iteration this tool call belongs to
}

export interface Message {
  id: string
  conversation_id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  attachments?: Attachment[]
  metrics?: Metrics
  parent_id?: string
  tool_calls?: ToolCall[]
  created_at: string
}

export interface Attachment {
  id: string
  message_id: string
  filename: string
  mime_type: string
  size: number
  path: string
  data?: string
}

export interface Metrics {
  input_tokens: number
  output_tokens: number
  total_tokens: number
  ttfb_ms: number
  total_latency_ms: number
  tokens_per_second: number
  cache_creation_input_tokens?: number
  cache_read_input_tokens?: number
}

export interface ProviderInfo {
  id: string
  name: string
  description: string
  type: 'cloud' | 'local'
  available: boolean
  has_api_key: boolean
}

// Model information from registry
export interface ModelInfo {
  id: string
  provider: string
  display_name: string
  family: string
  description: string
  pricing: {
    input_per_1m: number
    output_per_1m: number
  }
  context_window: number
  max_output: number
  capabilities: {
    thinking: boolean
    thinking_budget?: boolean
    tools: boolean
    vision: boolean
    citations?: boolean
    json: boolean
    streaming: boolean
    prompt_caching?: boolean
  }
  release_date?: string
  is_latest: boolean
  is_deprecated: boolean
  is_default: boolean
}

export interface PromptTemplate {
  id: string
  name: string
  description: string
  content: string
}

export interface StreamEvent {
  type: 'start' | 'delta' | 'thinking' | 'metrics' | 'done' | 'error' | 'debug' | 'user_message' | 'tool_start' | 'tool_complete' | 'tool_result' | 'tool_executing' | 'iteration_start' | 'iteration_end'
  content?: string
  metrics?: Metrics
  error?: string
  data?: unknown
  // Tool call fields
  tool_use_id?: string
  tool_name?: string
  tool_arguments?: Record<string, unknown>
  tool_result?: string
  // Iteration fields (ReAct)
  iteration?: number
  max_iterations?: number
  total_iterations?: number
  tool_count?: number
  has_more?: boolean
}

export interface DebugInfo {
  request?: {
    url: string
    method: string
    headers?: Record<string, string>
    body?: {
      model?: string
      messages?: unknown[]
      stream?: boolean
      think?: boolean
      tools?: unknown[]
      [key: string]: unknown
    }
    thinking_enabled?: boolean
  }
  response?: unknown
}

// MCP Types
export interface MCPTool {
  name: string
  description: string
  inputSchema?: Record<string, unknown>
}

export interface MCPServerStatus {
  name: string
  command: string
  args: string[]
  connected: boolean
  tools: MCPTool[]
  tool_count: number
}

export interface MCPStatus {
  enabled: boolean
  server_count: number
  total_tools: number
  servers: MCPServerStatus[]
}
