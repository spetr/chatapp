import { describe, it, expect } from 'vitest'
import type { Conversation, Message, Attachment, Metrics, ProviderInfo, PromptTemplate, StreamEvent, DebugInfo } from '@/types'

describe('TypeScript Types', () => {
  describe('Conversation', () => {
    it('should have correct shape', () => {
      const conversation: Conversation = {
        id: 'conv-123',
        title: 'Test Conversation',
        provider: 'claude',
        model: 'claude-sonnet-4-20250514',
        system_prompt: 'You are a helpful assistant',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      }

      expect(conversation.id).toBe('conv-123')
      expect(conversation.title).toBe('Test Conversation')
      expect(conversation.provider).toBe('claude')
    })
  })

  describe('Message', () => {
    it('should support all roles', () => {
      const userMessage: Message = {
        id: 'msg-1',
        conversation_id: 'conv-123',
        role: 'user',
        content: 'Hello',
        created_at: '2024-01-01T00:00:00Z',
      }

      const assistantMessage: Message = {
        id: 'msg-2',
        conversation_id: 'conv-123',
        role: 'assistant',
        content: 'Hi there!',
        created_at: '2024-01-01T00:00:01Z',
      }

      const systemMessage: Message = {
        id: 'msg-3',
        conversation_id: 'conv-123',
        role: 'system',
        content: 'System message',
        created_at: '2024-01-01T00:00:02Z',
      }

      expect(userMessage.role).toBe('user')
      expect(assistantMessage.role).toBe('assistant')
      expect(systemMessage.role).toBe('system')
    })

    it('should support optional fields', () => {
      const messageWithMetrics: Message = {
        id: 'msg-1',
        conversation_id: 'conv-123',
        role: 'assistant',
        content: 'Response',
        metrics: {
          input_tokens: 100,
          output_tokens: 50,
          total_tokens: 150,
          ttfb_ms: 200,
          total_latency_ms: 1000,
          tokens_per_second: 50,
        },
        parent_id: 'parent-123',
        created_at: '2024-01-01T00:00:00Z',
      }

      expect(messageWithMetrics.metrics?.total_tokens).toBe(150)
      expect(messageWithMetrics.parent_id).toBe('parent-123')
    })
  })

  describe('Attachment', () => {
    it('should have correct shape', () => {
      const attachment: Attachment = {
        id: 'att-123',
        message_id: 'msg-123',
        filename: 'image.png',
        mime_type: 'image/png',
        size: 1024,
        path: '/uploads/image.png',
        data: 'base64encodeddata...',
      }

      expect(attachment.filename).toBe('image.png')
      expect(attachment.mime_type).toBe('image/png')
    })
  })

  describe('Metrics', () => {
    it('should support caching fields', () => {
      const metrics: Metrics = {
        input_tokens: 1000,
        output_tokens: 500,
        total_tokens: 1500,
        ttfb_ms: 150,
        total_latency_ms: 2000,
        tokens_per_second: 250,
        cache_creation_input_tokens: 800,
        cache_read_input_tokens: 200,
      }

      expect(metrics.cache_creation_input_tokens).toBe(800)
      expect(metrics.cache_read_input_tokens).toBe(200)
    })
  })

  describe('ProviderInfo', () => {
    it('should have correct shape', () => {
      const provider: ProviderInfo = {
        id: 'anthropic',
        name: 'Anthropic',
        description: 'Claude models from Anthropic',
        type: 'cloud',
        available: true,
        has_api_key: true,
      }

      expect(provider.id).toBe('anthropic')
      expect(provider.type).toBe('cloud')
      expect(provider.available).toBe(true)
    })
  })

  describe('PromptTemplate', () => {
    it('should have correct shape', () => {
      const template: PromptTemplate = {
        id: 'coding',
        name: 'Coding Assistant',
        description: 'Helps with programming tasks',
        content: 'You are an expert programmer...',
      }

      expect(template.name).toBe('Coding Assistant')
    })
  })

  describe('StreamEvent', () => {
    it('should support all event types', () => {
      const startEvent: StreamEvent = { type: 'start' }
      const deltaEvent: StreamEvent = { type: 'delta', content: 'Hello' }
      const metricsEvent: StreamEvent = {
        type: 'metrics',
        metrics: {
          input_tokens: 100,
          output_tokens: 50,
          total_tokens: 150,
          ttfb_ms: 100,
          total_latency_ms: 500,
          tokens_per_second: 100,
        },
      }
      const doneEvent: StreamEvent = { type: 'done' }
      const errorEvent: StreamEvent = { type: 'error', error: 'Something went wrong' }
      const debugEvent: StreamEvent = { type: 'debug', data: { request: {} } }
      const userMessageEvent: StreamEvent = { type: 'user_message', content: 'User message' }

      expect(startEvent.type).toBe('start')
      expect(deltaEvent.content).toBe('Hello')
      expect(metricsEvent.metrics?.total_tokens).toBe(150)
      expect(doneEvent.type).toBe('done')
      expect(errorEvent.error).toBe('Something went wrong')
      expect(debugEvent.data).toBeDefined()
      expect(userMessageEvent.type).toBe('user_message')
    })
  })

  describe('DebugInfo', () => {
    it('should have correct shape', () => {
      const debugInfo: DebugInfo = {
        request: {
          url: 'https://api.anthropic.com/v1/messages',
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-API-Key': '***',
          },
          body: { model: 'claude-sonnet-4-20250514', messages: [] },
        },
        response: { id: 'msg-123' },
      }

      expect(debugInfo.request?.url).toContain('anthropic')
      expect(debugInfo.request?.method).toBe('POST')
    })
  })
})
