import type { Conversation, ConversationSettings, Message, ProviderInfo, PromptTemplate, Attachment, MCPStatus, ModelInfo } from '@/types'

const API_BASE = '/api'

async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  })

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(error.error || `HTTP ${response.status}`)
  }

  return response.json()
}

// Providers
export async function getProviders(): Promise<ProviderInfo[]> {
  const result = await fetchAPI<ProviderInfo[] | null>('/providers')
  return result || []
}

export async function getPrompts(): Promise<PromptTemplate[]> {
  const result = await fetchAPI<PromptTemplate[] | null>('/prompts')
  return result || []
}

// Models
export async function getModels(provider?: string): Promise<ModelInfo[]> {
  const params = provider ? `?provider=${provider}` : ''
  const result = await fetchAPI<ModelInfo[] | null>(`/models${params}`)
  return result || []
}

// Conversations
export async function getConversations(limit = 50, offset = 0): Promise<Conversation[]> {
  const result = await fetchAPI<Conversation[] | null>(`/conversations?limit=${limit}&offset=${offset}`)
  return result || []
}

export async function getConversation(id: string): Promise<Conversation> {
  return fetchAPI(`/conversations/${id}`)
}

export async function createConversation(data: {
  title?: string
  provider: string
  model: string
  system_prompt?: string
  settings?: ConversationSettings
}): Promise<Conversation> {
  return fetchAPI('/conversations', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export async function updateConversation(
  id: string,
  data: {
    title?: string
    model?: string
    system_prompt?: string
    settings?: ConversationSettings
  }
): Promise<Conversation> {
  return fetchAPI(`/conversations/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export async function deleteConversation(id: string): Promise<void> {
  await fetch(`${API_BASE}/conversations/${id}`, { method: 'DELETE' })
}

export async function exportConversation(id: string, format: 'json' | 'markdown' = 'json'): Promise<string> {
  const response = await fetch(`${API_BASE}/conversations/${id}/export?format=${format}`)
  if (format === 'markdown') {
    return response.text()
  }
  return response.json()
}

// Transform backend tool call format to frontend format
interface BackendToolCall {
  id: string
  name: string
  arguments: Record<string, unknown>
  result?: string
  is_error?: boolean
}

function transformToolCalls(toolCalls: BackendToolCall[] | undefined): Message['tool_calls'] {
  if (!toolCalls || toolCalls.length === 0) return undefined

  return toolCalls.map(tc => ({
    id: tc.id,
    name: tc.name,
    arguments: tc.arguments,
    result: tc.result,
    error: tc.is_error ? tc.result : undefined,
    status: tc.is_error ? 'error' as const : (tc.result ? 'completed' as const : 'pending' as const),
  }))
}

// Messages
export async function getMessages(conversationId: string, parentId?: string): Promise<Message[]> {
  const params = parentId ? `?parent_id=${parentId}` : ''
  const result = await fetchAPI<(Omit<Message, 'tool_calls'> & { tool_calls?: BackendToolCall[] })[] | null>(
    `/conversations/${conversationId}/messages${params}`
  )

  if (!result) return []

  // Transform tool_calls from backend format to frontend format
  return result.map(msg => ({
    ...msg,
    tool_calls: transformToolCalls(msg.tool_calls),
  }))
}

export function sendMessageStream(
  conversationId: string,
  content: string,
  attachments: string[] = [],
  parentId?: string
): EventSource {
  // We need to use fetch for POST with EventSource-like behavior
  // Create a custom EventSource-like object
  const url = `${API_BASE}/conversations/${conversationId}/messages`
  const body = JSON.stringify({ content, attachments, parent_id: parentId })

  // Return a simple object that mimics EventSource
  const eventSource = new MessageStream(url, body)
  return eventSource as unknown as EventSource
}

export async function stopGeneration(conversationId: string, streamId: string): Promise<void> {
  await fetch(`${API_BASE}/conversations/${conversationId}/stop?stream_id=${streamId}`, { method: 'POST' })
}

export function regenerateMessage(conversationId: string, messageId: string): EventSource {
  const url = `${API_BASE}/conversations/${conversationId}/regenerate`
  const body = JSON.stringify({ message_id: messageId })
  return new MessageStream(url, body) as unknown as EventSource
}

// Files
export async function uploadFile(file: File): Promise<Attachment> {
  const formData = new FormData()
  formData.append('file', file)

  const response = await fetch(`${API_BASE}/upload`, {
    method: 'POST',
    body: formData,
  })

  if (!response.ok) {
    throw new Error('Upload failed')
  }

  return response.json()
}

// MCP
export async function getMCPTools(): Promise<unknown[]> {
  return fetchAPI('/mcp/tools')
}

export async function getMCPStatus(): Promise<MCPStatus> {
  return fetchAPI('/mcp/status')
}

// Context Management
export interface ContextStats {
  message_count: number
  estimated_tokens: number
  max_tokens: number
  token_percent_used: number
  needs_optimization: boolean
  status: 'ok' | 'info' | 'warning' | 'critical'
  max_messages: number
  estimated_input_cost: number
  caching_enabled: boolean
  recommendations: string[]
}

export interface MessageBreakdown {
  id: string
  role: string
  tokens: number
  percent: number
  content_preview: string
  has_attachments: boolean
  attachment_count: number
  created_at?: string
}

export interface ContextBreakdown {
  total_tokens: number
  system_tokens: number
  message_count: number
  breakdown: MessageBreakdown[]
}

export interface CompactRequest {
  strategy: 'summarize' | 'drop_oldest' | 'smart'
  keep_recent: number
  preview_only: boolean
}

export interface CompactResult {
  status: 'preview' | 'applied' | 'no_change'
  message?: string
  original_tokens: number
  new_tokens: number
  tokens_saved: number
  percent_saved: number
  messages_removed: number
  messages_kept: number
  summary: string
  strategy: string
}

export interface ContextPreview {
  messages: { role: string; content: string; tokens: number }[]
  total_tokens: number
  message_count: number
  was_truncated: boolean
  original_count: number
  max_history_length: number
}

export async function getContextStats(conversationId: string): Promise<ContextStats> {
  return fetchAPI(`/conversations/${conversationId}/context-stats`)
}

export async function getContextBreakdown(conversationId: string): Promise<ContextBreakdown> {
  return fetchAPI(`/conversations/${conversationId}/context-breakdown`)
}

export async function compactContext(conversationId: string, options: CompactRequest): Promise<CompactResult> {
  return fetchAPI(`/conversations/${conversationId}/context-compact`, {
    method: 'POST',
    body: JSON.stringify(options),
  })
}

export async function getContextPreview(conversationId: string): Promise<ContextPreview> {
  return fetchAPI(`/conversations/${conversationId}/context-preview`)
}

// Custom MessageStream class for POST with SSE
class MessageStream {
  private controller: AbortController
  private listeners: Map<string, ((event: MessageEvent) => void)[]> = new Map()
  public readyState: number = 0

  constructor(url: string, body: string) {
    this.controller = new AbortController()
    this.start(url, body)
  }

  private async start(url: string, body: string) {
    try {
      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body,
        signal: this.controller.signal,
      })

      if (!response.ok || !response.body) {
        this.emit('error', new MessageEvent('error', { data: 'Connection failed' }))
        return
      }

      this.readyState = 1
      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      let currentEventType = 'message'

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          if (line.startsWith('event: ')) {
            currentEventType = line.slice(7).trim()
            continue
          }
          if (line.startsWith('data: ')) {
            const data = line.slice(6)
            try {
              const parsed = JSON.parse(data)
              this.emit('message', new MessageEvent('message', { data }))

              // Emit typed event based on SSE event type or parsed type
              const eventType = parsed.type || currentEventType
              if (eventType && eventType !== 'message') {
                this.emit(eventType, new MessageEvent(eventType, { data }))
              }
            } catch {
              // Not JSON, skip
            }
            currentEventType = 'message' // Reset after processing
          }
        }
      }

      this.readyState = 2
    } catch (error) {
      if ((error as Error).name !== 'AbortError') {
        this.emit('error', new MessageEvent('error', { data: String(error) }))
      }
    }
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, [])
    }
    this.listeners.get(type)!.push(listener)
  }

  removeEventListener(type: string, listener: (event: MessageEvent) => void) {
    const listeners = this.listeners.get(type)
    if (listeners) {
      const index = listeners.indexOf(listener)
      if (index > -1) {
        listeners.splice(index, 1)
      }
    }
  }

  private emit(type: string, event: MessageEvent) {
    const listeners = this.listeners.get(type)
    if (listeners) {
      for (const listener of listeners) {
        listener(event)
      }
    }
  }

  close() {
    this.controller.abort()
    this.readyState = 2
  }
}
