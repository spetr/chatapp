import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useChatStore } from '@/stores/chat'
import * as api from '@/api/client'

// Mock the API module
vi.mock('@/api/client', () => ({
  getProviders: vi.fn(),
  getModels: vi.fn(),
  getPrompts: vi.fn(),
  getConversations: vi.fn(),
  getConversation: vi.fn(),
  getMessages: vi.fn(),
  createConversation: vi.fn(),
  deleteConversation: vi.fn(),
  updateConversation: vi.fn(),
  sendMessageStream: vi.fn(),
  stopGeneration: vi.fn(),
  regenerateMessage: vi.fn(),
}))

describe('Chat Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  describe('Initial State', () => {
    it('should have empty initial state', () => {
      const store = useChatStore()

      expect(store.conversations).toEqual([])
      expect(store.currentConversation).toBeNull()
      expect(store.messages).toEqual([])
      expect(store.providers).toEqual([])
      expect(store.prompts).toEqual([])
      expect(store.isLoading).toBe(false)
      expect(store.isStreaming).toBe(false)
      expect(store.streamingContent).toBe('')
    })
  })

  describe('loadProviders', () => {
    it('should load providers, models, and prompts', async () => {
      const mockProviders = [
        { id: 'anthropic', name: 'Anthropic', description: 'Claude models', type: 'cloud' as const, available: true, has_api_key: true },
        { id: 'openai', name: 'OpenAI', description: 'GPT models', type: 'cloud' as const, available: true, has_api_key: true },
      ]
      const mockModels = [
        { id: 'claude-sonnet-4-5-20250929', provider: 'anthropic', display_name: 'Claude Sonnet 4.5', family: 'sonnet-4.5', description: 'Test', pricing: { input_per_1m: 3, output_per_1m: 15 }, context_window: 200000, max_output: 64000, capabilities: { thinking: true, tools: true, vision: true, json: true, streaming: true }, is_latest: true, is_deprecated: false, is_default: true },
      ]
      const mockPrompts = [
        { id: 'default', name: 'Default', description: 'Default prompt', content: 'You are helpful' },
      ]

      vi.mocked(api.getProviders).mockResolvedValue(mockProviders)
      vi.mocked(api.getModels).mockResolvedValue(mockModels)
      vi.mocked(api.getPrompts).mockResolvedValue(mockPrompts)

      const store = useChatStore()
      await store.loadProviders()

      expect(store.providers).toEqual(mockProviders)
      expect(store.models).toEqual(mockModels)
      expect(store.prompts).toEqual(mockPrompts)
    })

    it('should handle errors gracefully', async () => {
      vi.mocked(api.getProviders).mockRejectedValue(new Error('Network error'))
      vi.mocked(api.getModels).mockRejectedValue(new Error('Network error'))
      vi.mocked(api.getPrompts).mockRejectedValue(new Error('Network error'))

      const store = useChatStore()
      await store.loadProviders()

      expect(store.providers).toEqual([])
      expect(store.models).toEqual([])
    })
  })

  describe('loadConversations', () => {
    it('should load conversations', async () => {
      const mockConversations = [
        { id: '1', title: 'Test', provider: 'claude', model: 'claude-sonnet-4-20250514', system_prompt: '', created_at: new Date().toISOString(), updated_at: new Date().toISOString() },
      ]

      vi.mocked(api.getConversations).mockResolvedValue(mockConversations)

      const store = useChatStore()
      await store.loadConversations()

      expect(store.conversations).toEqual(mockConversations)
    })
  })

  describe('loadConversation', () => {
    it('should load a specific conversation with messages', async () => {
      const mockConv = { id: '1', title: 'Test', provider: 'claude', model: 'claude-sonnet-4-20250514', system_prompt: '', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }
      const mockMessages = [
        { id: 'm1', conversation_id: '1', role: 'user' as const, content: 'Hello', created_at: new Date().toISOString() },
        { id: 'm2', conversation_id: '1', role: 'assistant' as const, content: 'Hi there!', created_at: new Date().toISOString() },
      ]

      vi.mocked(api.getConversation).mockResolvedValue(mockConv)
      vi.mocked(api.getMessages).mockResolvedValue(mockMessages)

      const store = useChatStore()
      await store.loadConversation('1')

      expect(store.currentConversation).toEqual(mockConv)
      expect(store.messages).toEqual(mockMessages)
      expect(store.isLoading).toBe(false)
    })

    it('should handle empty messages', async () => {
      const mockConv = { id: '1', title: 'Test', provider: 'claude', model: 'claude-sonnet-4-20250514', system_prompt: '', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }

      vi.mocked(api.getConversation).mockResolvedValue(mockConv)
      vi.mocked(api.getMessages).mockResolvedValue([])

      const store = useChatStore()
      await store.loadConversation('1')

      expect(store.messages).toEqual([])
    })

    it('should throw error on failure', async () => {
      vi.mocked(api.getConversation).mockRejectedValue(new Error('Not found'))

      const store = useChatStore()
      await expect(store.loadConversation('nonexistent')).rejects.toThrow()
    })
  })

  describe('createConversation', () => {
    it('should create a new conversation', async () => {
      const mockConv = { id: 'new-1', title: 'New Chat', provider: 'claude', model: 'claude-sonnet-4-20250514', system_prompt: 'You are helpful', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }

      vi.mocked(api.createConversation).mockResolvedValue(mockConv)

      const store = useChatStore()
      const result = await store.createConversation('claude', 'claude-sonnet-4-20250514', 'You are helpful')

      expect(result).toEqual(mockConv)
      expect(store.currentConversation).toEqual(mockConv)
      expect(store.conversations.length).toBe(1)
      expect(store.conversations[0].id).toBe('new-1')
      expect(store.messages).toEqual([])
    })
  })

  describe('deleteConversation', () => {
    it('should delete a conversation', async () => {
      vi.mocked(api.deleteConversation).mockResolvedValue(undefined)

      const store = useChatStore()
      store.conversations = [
        { id: '1', title: 'Test', provider: 'claude', model: 'claude-sonnet-4-20250514', system_prompt: '', created_at: new Date().toISOString(), updated_at: new Date().toISOString() },
        { id: '2', title: 'Test 2', provider: 'claude', model: 'claude-sonnet-4-20250514', system_prompt: '', created_at: new Date().toISOString(), updated_at: new Date().toISOString() },
      ]

      await store.deleteConversation('1')

      expect(store.conversations.length).toBe(1)
      expect(store.conversations[0].id).toBe('2')
    })

    it('should clear current if deleted', async () => {
      vi.mocked(api.deleteConversation).mockResolvedValue(undefined)

      const store = useChatStore()
      const conv = { id: '1', title: 'Test', provider: 'claude', model: 'claude-sonnet-4-20250514', system_prompt: '', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }
      store.conversations = [conv]
      store.currentConversation = conv
      store.messages = [{ id: 'm1', conversation_id: '1', role: 'user', content: 'Hello', created_at: new Date().toISOString() }]

      await store.deleteConversation('1')

      expect(store.currentConversation).toBeNull()
      expect(store.messages).toEqual([])
    })
  })

  describe('clearCurrentConversation', () => {
    it('should clear all current state', () => {
      const store = useChatStore()
      store.currentConversation = { id: '1', title: 'Test', provider: 'claude', model: 'claude-sonnet-4-20250514', system_prompt: '', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }
      store.messages = [{ id: 'm1', conversation_id: '1', role: 'user', content: 'Hello', created_at: new Date().toISOString() }]
      store.streamingContent = 'test'
      store.currentMetrics = { input_tokens: 100, output_tokens: 50, total_tokens: 150, ttfb_ms: 100, total_latency_ms: 500, tokens_per_second: 50 }
      store.debugInfo = { request: { url: '', method: '', headers: {}, body: {} }, response: {} }

      store.clearCurrentConversation()

      expect(store.currentConversation).toBeNull()
      expect(store.messages).toEqual([])
      expect(store.streamingContent).toBe('')
      expect(store.currentMetrics).toBeNull()
      expect(store.debugInfo).toBeNull()
    })
  })

  describe('computed: totalTokensUsed', () => {
    it('should calculate total tokens from messages', () => {
      const store = useChatStore()
      store.messages = [
        { id: 'm1', conversation_id: '1', role: 'user', content: 'Hello', created_at: new Date().toISOString() },
        { id: 'm2', conversation_id: '1', role: 'assistant', content: 'Hi!', metrics: { input_tokens: 10, output_tokens: 5, total_tokens: 15, ttfb_ms: 0, total_latency_ms: 0, tokens_per_second: 0 }, created_at: new Date().toISOString() },
        { id: 'm3', conversation_id: '1', role: 'user', content: 'How are you?', created_at: new Date().toISOString() },
        { id: 'm4', conversation_id: '1', role: 'assistant', content: 'Good!', metrics: { input_tokens: 20, output_tokens: 5, total_tokens: 25, ttfb_ms: 0, total_latency_ms: 0, tokens_per_second: 0 }, created_at: new Date().toISOString() },
      ]

      expect(store.totalTokensUsed).toBe(40)
    })

    it('should return 0 for empty messages', () => {
      const store = useChatStore()
      expect(store.totalTokensUsed).toBe(0)
    })
  })

  describe('computed: currentProvider', () => {
    it('should return current provider info', () => {
      const store = useChatStore()
      store.providers = [
        { id: 'anthropic', name: 'Anthropic', description: 'Claude models', type: 'cloud', available: true, has_api_key: true },
        { id: 'openai', name: 'OpenAI', description: 'GPT models', type: 'cloud', available: true, has_api_key: true },
      ]
      store.currentConversation = { id: '1', title: 'Test', provider: 'anthropic', model: 'claude-sonnet-4-5-20250929', system_prompt: '', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }

      expect(store.currentProvider?.id).toBe('anthropic')
    })

    it('should return null if no current conversation', () => {
      const store = useChatStore()
      expect(store.currentProvider).toBeNull()
    })
  })
})
