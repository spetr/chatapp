import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Conversation, ConversationSettings, Message, ProviderInfo, PromptTemplate, Metrics, DebugInfo, ModelInfo, ToolCall } from '@/types'
import * as api from '@/api/client'

export const useChatStore = defineStore('chat', () => {
  // State
  const conversations = ref<Conversation[]>([])
  const currentConversation = ref<Conversation | null>(null)
  const messages = ref<Message[]>([])
  const providers = ref<ProviderInfo[]>([])
  const models = ref<ModelInfo[]>([])
  const prompts = ref<PromptTemplate[]>([])

  const isLoading = ref(false)
  const isStreaming = ref(false)
  const streamingContent = ref('')
  const streamingThinking = ref('')
  const streamingToolCalls = ref<ToolCall[]>([])
  const streamId = ref<string | null>(null)
  const currentMetrics = ref<Metrics | null>(null)
  const debugInfo = ref<DebugInfo | null>(null)

  // Flag to prevent duplicate message handling
  const streamFinalized = ref(false)

  // Computed
  const currentProvider = computed(() => {
    if (!currentConversation.value) return null
    return providers.value.find((p) => p.id === currentConversation.value?.provider)
  })

  const totalTokensUsed = computed(() => {
    return messages.value.reduce((sum, msg) => {
      return sum + (msg.metrics?.total_tokens || 0)
    }, 0)
  })

  // Computed helpers for models
  const getModelsForProvider = computed(() => {
    return (providerId: string) => models.value.filter(m => m.provider === providerId)
  })

  const getDefaultModel = computed(() => {
    return (providerId: string) => models.value.find(m => m.provider === providerId && m.is_default)
  })

  // Actions
  async function loadProviders() {
    try {
      const [providersResult, modelsResult, promptsResult] = await Promise.all([
        api.getProviders(),
        api.getModels(),
        api.getPrompts(),
      ])
      providers.value = providersResult || []
      models.value = modelsResult || []
      prompts.value = promptsResult || []
    } catch (error) {
      console.error('Failed to load providers/models:', error)
      providers.value = []
      models.value = []
      prompts.value = []
    }
  }

  async function loadConversations() {
    try {
      const result = await api.getConversations()
      conversations.value = result || []
    } catch (error) {
      console.error('Failed to load conversations:', error)
      conversations.value = []
    }
  }

  async function loadConversation(id: string) {
    isLoading.value = true
    try {
      currentConversation.value = await api.getConversation(id)
      const loadedMessages = await api.getMessages(id)
      messages.value = loadedMessages || []
    } catch (error) {
      console.error('Failed to load conversation:', error)
      throw error
    } finally {
      isLoading.value = false
    }
  }

  async function createConversation(
    provider: string,
    model: string,
    systemPrompt?: string,
    settings?: ConversationSettings
  ) {
    try {
      const conv = await api.createConversation({
        provider,
        model,
        system_prompt: systemPrompt,
        settings,
      })
      conversations.value.unshift(conv)
      currentConversation.value = conv
      messages.value = []
      return conv
    } catch (error) {
      console.error('Failed to create conversation:', error)
      throw error
    }
  }

  async function deleteConversation(id: string) {
    try {
      await api.deleteConversation(id)
      conversations.value = conversations.value.filter((c) => c.id !== id)
      if (currentConversation.value?.id === id) {
        currentConversation.value = null
        messages.value = []
      }
    } catch (error) {
      console.error('Failed to delete conversation:', error)
      throw error
    }
  }

  async function sendMessage(content: string, attachments: string[] = []) {
    if (!currentConversation.value || isStreaming.value) return

    // Reset all streaming state
    isStreaming.value = true
    streamFinalized.value = false
    streamingContent.value = ''
    streamingThinking.value = ''
    streamingToolCalls.value = []
    currentMetrics.value = null
    debugInfo.value = null

    try {
      const stream = api.sendMessageStream(currentConversation.value.id, content, attachments)

      stream.addEventListener('message', (event: MessageEvent) => {
        try {
          const data = JSON.parse(event.data)
          handleStreamEvent(data)
        } catch {
          // Ignore parse errors
        }
      })

      // Wait for stream to complete
      await new Promise<void>((resolve, reject) => {
        const checkComplete = setInterval(() => {
          if ((stream as unknown as { readyState: number }).readyState === 2) {
            clearInterval(checkComplete)
            resolve()
          }
        }, 100)

        stream.addEventListener('error', () => {
          clearInterval(checkComplete)
          reject(new Error('Stream error'))
        })
      })
    } catch (error) {
      console.error('Failed to send message:', error)
      throw error
    } finally {
      isStreaming.value = false
      streamId.value = null
    }
  }

  // Helper to extract tool ID from various event structures
  function extractToolId(event: Record<string, unknown>): string {
    // Backend sends 'id' directly in the event object
    // Check direct properties first, then nested data
    return String(
      event.id ||
      event.tool_use_id ||
      (event.data as Record<string, unknown> | undefined)?.id ||
      ''
    )
  }

  // Helper to find tool call by ID
  function findToolCallIndex(toolId: string): number {
    return streamingToolCalls.value.findIndex(t => t.id === toolId)
  }

  // Helper to clear streaming state
  function clearStreamingState() {
    streamingContent.value = ''
    streamingThinking.value = ''
    streamingToolCalls.value = []
  }

  // Helper to create final message content
  function buildFinalContent(): string {
    if (streamingThinking.value) {
      return `<think>${streamingThinking.value}</think>\n\n${streamingContent.value}`
    }
    return streamingContent.value
  }

  function handleStreamEvent(event: Record<string, unknown>) {
    const eventType = event.type as string

    // Ignore events after stream has been finalized (prevents duplicates)
    if (streamFinalized.value && (eventType === 'done' || eventType === 'error')) {
      console.warn('Ignoring duplicate finalization event:', eventType)
      return
    }

    switch (eventType) {
      case 'user_message': {
        const userMessage: Message = {
          id: String(event.id || Date.now()),
          conversation_id: String(event.conversation_id || currentConversation.value?.id || ''),
          role: 'user',
          content: String(event.content || ''),
          created_at: String(event.created_at || new Date().toISOString()),
        }
        messages.value.push(userMessage)
        break
      }

      case 'start':
        clearStreamingState()
        break

      case 'tool_start': {
        const data = (event.data as Record<string, unknown>) || event
        const toolId = String(data.id || event.tool_use_id || `tool_${Date.now()}`)
        const toolCall: ToolCall = {
          id: toolId,
          name: String(data.name || event.tool_name || 'unknown'),
          arguments: (data.arguments as Record<string, unknown>) || {},
          status: 'running',
          started_at: new Date().toISOString(),
        }
        streamingToolCalls.value = [...streamingToolCalls.value, toolCall]
        break
      }

      case 'tool_complete': {
        const toolId = extractToolId(event)
        const idx = findToolCallIndex(toolId)
        if (idx !== -1) {
          const data = (event.data as Record<string, unknown>) || event
          const args = data.arguments as Record<string, unknown>
          if (args) {
            // Create new array for reactivity
            streamingToolCalls.value = streamingToolCalls.value.map((tc, i) =>
              i === idx ? { ...tc, arguments: args } : tc
            )
          }
        }
        break
      }

      case 'tool_executing': {
        const toolId = extractToolId(event)
        const idx = findToolCallIndex(toolId)
        if (idx !== -1) {
          // Create new array for reactivity
          streamingToolCalls.value = streamingToolCalls.value.map((tc, i) =>
            i === idx ? { ...tc, status: 'running' as const } : tc
          )
        }
        break
      }

      case 'tool_result': {
        const toolId = extractToolId(event)
        const idx = findToolCallIndex(toolId)

        if (idx !== -1) {
          const resultContent = String(event.content || event.tool_result || '')
          const isError = Boolean(event.is_error)

          // Create new array to ensure Vue reactivity triggers
          streamingToolCalls.value = streamingToolCalls.value.map((tc, i) => {
            if (i === idx) {
              return {
                ...tc,
                result: resultContent,
                status: isError ? 'error' as const : 'completed' as const,
                error: isError ? resultContent : undefined,
                completed_at: new Date().toISOString(),
              }
            }
            return tc
          })
        }
        break
      }

      case 'thinking':
        streamingThinking.value += String(event.content || '')
        break

      case 'delta':
        streamingContent.value += String(event.content || '')
        break

      case 'metrics':
        currentMetrics.value = (event.metrics as Metrics) || null
        break

      case 'debug':
        debugInfo.value = event.data as DebugInfo
        break

      case 'done': {
        // Mark as finalized to prevent duplicate handling
        streamFinalized.value = true

        const assistantMessage: Message = {
          id: String(event.message_id || Date.now()),
          conversation_id: currentConversation.value?.id || '',
          role: 'assistant',
          content: buildFinalContent(),
          metrics: currentMetrics.value || undefined,
          tool_calls: streamingToolCalls.value.length > 0 ? [...streamingToolCalls.value] : undefined,
          created_at: new Date().toISOString(),
        }
        messages.value = [...messages.value, assistantMessage]
        clearStreamingState()

        // Update conversation list in background
        loadConversations()
        break
      }

      case 'error': {
        // Mark as finalized to prevent duplicate handling
        streamFinalized.value = true

        console.error('Stream error:', event.error)

        const errorMessage: Message = {
          id: String(Date.now()),
          conversation_id: currentConversation.value?.id || '',
          role: 'assistant',
          content: `⚠️ **Chyba při generování odpovědi**\n\n${event.error || 'Neznámá chyba'}\n\nZkuste odpověď vygenerovat znovu pomocí tlačítka "Vygenerovat znovu".`,
          tool_calls: streamingToolCalls.value.length > 0 ? [...streamingToolCalls.value] : undefined,
          created_at: new Date().toISOString(),
        }
        messages.value = [...messages.value, errorMessage]
        clearStreamingState()
        break
      }
    }
  }

  async function stopGeneration() {
    if (streamId.value && currentConversation.value) {
      await api.stopGeneration(currentConversation.value.id, streamId.value)
      isStreaming.value = false
      streamId.value = null
    }
  }

  async function regenerateLastMessage() {
    if (!currentConversation.value || messages.value.length < 2) return

    // Find last assistant message
    let lastAssistantIdx = -1
    for (let i = messages.value.length - 1; i >= 0; i--) {
      if (messages.value[i].role === 'assistant') {
        lastAssistantIdx = i
        break
      }
    }
    if (lastAssistantIdx === -1) return

    const messageToRegenerate = messages.value[lastAssistantIdx]

    // Reset all streaming state
    isStreaming.value = true
    streamFinalized.value = false
    streamingContent.value = ''
    streamingThinking.value = ''
    streamingToolCalls.value = []
    currentMetrics.value = null
    debugInfo.value = null

    try {
      // Remove the last assistant message from view
      messages.value = messages.value.slice(0, lastAssistantIdx)

      const stream = api.regenerateMessage(currentConversation.value.id, messageToRegenerate.id)

      stream.addEventListener('message', (event: MessageEvent) => {
        try {
          const data = JSON.parse(event.data)
          handleStreamEvent(data)
        } catch {
          // Ignore
        }
      })

      // Wait for stream to complete
      await new Promise<void>((resolve, reject) => {
        const checkComplete = setInterval(() => {
          if ((stream as unknown as { readyState: number }).readyState === 2) {
            clearInterval(checkComplete)
            resolve()
          }
        }, 100)

        stream.addEventListener('error', () => {
          clearInterval(checkComplete)
          reject(new Error('Stream error'))
        })
      })
    } catch (error) {
      console.error('Failed to regenerate:', error)
      // Restore the message
      messages.value.push(messageToRegenerate)
      throw error
    } finally {
      isStreaming.value = false
      streamId.value = null
    }
  }

  async function updateConversationSettings(
    title?: string,
    model?: string,
    systemPrompt?: string,
    settings?: ConversationSettings
  ) {
    if (!currentConversation.value) return

    try {
      currentConversation.value = await api.updateConversation(currentConversation.value.id, {
        title,
        model,
        system_prompt: systemPrompt,
        settings,
      })

      // Update in conversations list
      const idx = conversations.value.findIndex(c => c.id === currentConversation.value?.id)
      if (idx !== -1) {
        conversations.value[idx] = currentConversation.value
      }
    } catch (error) {
      console.error('Failed to update conversation:', error)
      throw error
    }
  }

  function clearCurrentConversation() {
    currentConversation.value = null
    messages.value = []
    streamFinalized.value = false
    streamingContent.value = ''
    streamingThinking.value = ''
    streamingToolCalls.value = []
    currentMetrics.value = null
    debugInfo.value = null
  }

  return {
    // State
    conversations,
    currentConversation,
    messages,
    providers,
    models,
    prompts,
    isLoading,
    isStreaming,
    streamingContent,
    streamingThinking,
    streamingToolCalls,
    currentMetrics,
    debugInfo,

    // Computed
    currentProvider,
    totalTokensUsed,
    getModelsForProvider,
    getDefaultModel,

    // Actions
    loadProviders,
    loadConversations,
    loadConversation,
    createConversation,
    deleteConversation,
    sendMessage,
    stopGeneration,
    regenerateLastMessage,
    updateConversationSettings,
    clearCurrentConversation,
  }
})
