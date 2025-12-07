<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useChatStore } from '@/stores/chat'
import type { ConversationSettings } from '@/types'
import { useToast } from 'primevue/usetoast'
import { getContextStats, type ContextStats } from '@/api/client'
import Sidebar from '@/components/Sidebar.vue'
import MessageBubble from '@/components/MessageBubble.vue'
import ChatInput from '@/components/ChatInput.vue'
import SettingsPanel from '@/components/SettingsPanel.vue'
import DebugPanel from '@/components/DebugPanel.vue'
import RuntimeBar from '@/components/RuntimeBar.vue'
import ContextManagerPanel from '@/components/ContextManagerPanel.vue'
import ProviderIcon from '@/components/ProviderIcon.vue'
import Button from 'primevue/button'
import ProgressSpinner from 'primevue/progressspinner'

const route = useRoute()
const router = useRouter()
const chatStore = useChatStore()
const toast = useToast()

const messagesContainer = ref<HTMLElement | null>(null)
const showSettings = ref(false)
const showDebug = ref(true)
const showContextManager = ref(false)
const contextStats = ref<ContextStats | null>(null)

// Load context stats for current conversation
async function loadContextStats() {
  if (!chatStore.currentConversation?.id) {
    contextStats.value = null
    return
  }
  try {
    contextStats.value = await getContextStats(chatStore.currentConversation.id)
  } catch (error) {
    console.error('Failed to load context stats:', error)
    contextStats.value = null
  }
}

// Context warning computed - based on actual context estimate, not historical usage
const showContextWarning = computed(() => {
  if (!contextStats.value) return false
  return contextStats.value.estimated_tokens > 50000
})

// Load conversation if ID in route
watch(
  () => route.params.id,
  async (id) => {
    if (id && typeof id === 'string') {
      try {
        await chatStore.loadConversation(id)
        await loadContextStats()
      } catch {
        toast.add({
          severity: 'error',
          summary: 'Chyba',
          detail: 'Nepodařilo se načíst konverzaci',
          life: 3000,
        })
        router.push('/')
      }
    } else {
      contextStats.value = null
    }
  },
  { immediate: true }
)

// Scroll to bottom helper
function scrollToBottom() {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
  }
}

// Scroll to bottom when messages array changes
watch(
  () => chatStore.messages,
  () => nextTick(scrollToBottom),
  { deep: true }
)

// Scroll during streaming - watch the actual content being streamed
watch(
  [
    () => chatStore.streamingContent,
    () => chatStore.streamingThinking,
    () => chatStore.streamingToolCalls,
    () => chatStore.isStreaming,
  ],
  () => nextTick(scrollToBottom),
  { flush: 'post' }
)

// Create streaming message for display (including thinking content)
// Returns a message object whenever streaming is active, even if no content yet
const streamingMessage = computed(() => {
  // Show placeholder immediately when streaming starts
  if (!chatStore.isStreaming) return null

  const hasThinking = chatStore.streamingThinking.length > 0
  const hasContent = chatStore.streamingContent.length > 0

  // Combine thinking and content for display
  let content = ''
  if (hasThinking) {
    content = `<think>${chatStore.streamingThinking}</think>`
  }
  if (hasContent) {
    content += (hasThinking ? '\n\n' : '') + chatStore.streamingContent
  }

  return {
    id: 'streaming',
    conversation_id: chatStore.currentConversation?.id || '',
    role: 'assistant' as const,
    content,
    created_at: new Date().toISOString(),
  }
})

async function handleSend(content: string, attachments: string[]) {
  if (!chatStore.currentConversation) {
    // Show settings to create new conversation first
    showSettings.value = true
    return
  }

  try {
    await chatStore.sendMessage(content, attachments)
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: 'Chyba',
      detail: 'Nepodařilo se odeslat zprávu',
      life: 3000,
    })
  }
}

async function handleCreateConversation(provider: string, model: string, systemPrompt: string, settings?: ConversationSettings) {
  try {
    const conv = await chatStore.createConversation(provider, model, systemPrompt, settings)
    router.push(`/chat/${conv.id}`)
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: 'Chyba',
      detail: 'Nepodařilo se vytvořit konverzaci',
      life: 3000,
    })
  }
}

async function handleSaveConversation(model: string, systemPrompt: string, settings?: ConversationSettings) {
  try {
    await chatStore.updateConversationSettings(undefined, model, systemPrompt, settings)
    toast.add({
      severity: 'success',
      summary: 'Uloženo',
      detail: 'Nastavení konverzace bylo aktualizováno',
      life: 2000,
    })
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: 'Chyba',
      detail: 'Nepodařilo se uložit nastavení',
      life: 3000,
    })
  }
}

async function handleContextCompacted() {
  if (chatStore.currentConversation?.id) {
    await chatStore.loadConversation(chatStore.currentConversation.id)
    await loadContextStats()
  }
}

function handleCopy(content: string) {
  navigator.clipboard.writeText(content)
  toast.add({
    severity: 'success',
    summary: 'Zkopírováno',
    detail: 'Zpráva zkopírována do schránky',
    life: 2000,
  })
}

async function handleRegenerate() {
  try {
    await chatStore.regenerateLastMessage()
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: 'Chyba',
      detail: 'Nepodařilo se vygenerovat zprávu znovu',
      life: 3000,
    })
  }
}

function handleFork(_messageId: string) {
  // TODO: Implement forking
  toast.add({
    severity: 'info',
    summary: 'Připravujeme',
    detail: 'Větvení bude dostupné v budoucí aktualizaci',
    life: 3000,
  })
}
</script>

<template>
  <div class="h-screen flex">
    <!-- Sidebar -->
    <Sidebar @new-chat="showSettings = true" />

    <!-- Main Content -->
    <div class="flex-1 flex flex-col min-w-0">
      <!-- Header -->
      <header class="h-12 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 flex items-center justify-between px-4">
        <!-- Left: Title -->
        <div class="flex items-center gap-3 min-w-0">
          <ProviderIcon :provider="chatStore.currentConversation?.provider || ''" :size="20" />
          <h1 v-if="chatStore.currentConversation" class="font-semibold truncate">
            {{ chatStore.currentConversation.title }}
          </h1>
          <h1 v-else class="text-gray-500">Nová konverzace</h1>
        </div>

        <!-- Right: Actions -->
        <div class="flex items-center gap-2 flex-shrink-0">
          <Button
            icon="pi pi-database"
            @click="showContextManager = true"
            text
            rounded
            severity="secondary"
            v-tooltip="'Správa kontextu'"
            :disabled="!chatStore.currentConversation"
          />
          <Button
            icon="pi pi-cog"
            @click="showSettings = true"
            text
            rounded
            severity="secondary"
            v-tooltip="'Nastavení'"
          />
          <Button
            :icon="showDebug ? 'pi pi-eye-slash' : 'pi pi-eye'"
            @click="showDebug = !showDebug"
            text
            rounded
            severity="secondary"
            v-tooltip="showDebug ? 'Skrýt ladění' : 'Zobrazit ladění'"
          />
        </div>
      </header>

      <!-- Runtime Bar (below header) -->
      <RuntimeBar
        :conversation="chatStore.currentConversation"
        :metrics="chatStore.currentMetrics"
        :debug-info="chatStore.debugInfo"
        :is-streaming="chatStore.isStreaming"
        :total-tokens="chatStore.totalTokensUsed"
      />

      <div class="flex-1 flex overflow-hidden">
        <!-- Chat Area -->
        <div class="flex-1 flex flex-col min-w-0">
          <!-- Messages -->
          <div
            ref="messagesContainer"
            class="flex-1 overflow-y-auto"
          >
            <!-- Loading state -->
            <div v-if="chatStore.isLoading" class="flex items-center justify-center h-full">
              <ProgressSpinner />
            </div>

            <!-- Empty state -->
            <div
              v-else-if="!chatStore.currentConversation && chatStore.messages.length === 0"
              class="flex flex-col items-center justify-center h-full"
            >
              <div class="text-center max-w-md px-6">
                <!-- Icon -->
                <div class="w-20 h-20 mx-auto mb-6 rounded-2xl bg-gradient-to-br from-blue-500/10 to-purple-500/10 dark:from-blue-500/20 dark:to-purple-500/20 flex items-center justify-center">
                  <i class="pi pi-comments text-4xl text-blue-500"></i>
                </div>

                <!-- Title -->
                <h2 class="text-2xl font-bold text-gray-900 dark:text-white mb-2">
                  Vítejte v ChatApp
                </h2>

                <!-- Description -->
                <p class="text-gray-500 dark:text-gray-400 mb-6">
                  Klikněte na <span class="font-medium text-blue-500">„Nová konverzace"</span> v postranním panelu pro zahájení chatu s AI.
                </p>

                <!-- Features hint -->
                <div class="flex flex-wrap justify-center gap-3 text-xs text-gray-400 dark:text-gray-500">
                  <span class="flex items-center gap-1">
                    <i class="pi pi-sparkles text-orange-400"></i> Claude
                  </span>
                  <span class="flex items-center gap-1">
                    <i class="pi pi-bolt text-green-400"></i> OpenAI
                  </span>
                  <span class="flex items-center gap-1">
                    <i class="pi pi-server text-blue-400"></i> Ollama
                  </span>
                </div>
              </div>
            </div>

            <!-- Messages list -->
            <div v-else>
              <MessageBubble
                v-for="message in chatStore.messages"
                :key="message.id"
                :message="message"
                @copy="handleCopy"
                @regenerate="handleRegenerate"
                @fork="handleFork"
              />

              <!-- Streaming message -->
              <MessageBubble
                v-if="streamingMessage"
                :message="streamingMessage"
                :is-streaming="true"
                :streaming-tool-calls="chatStore.streamingToolCalls"
              />

              <!-- Token warning -->
              <div
                v-if="showContextWarning && contextStats"
                class="mx-4 my-2 p-3 bg-yellow-50 dark:bg-yellow-900/20 rounded-lg text-sm text-yellow-800 dark:text-yellow-200 flex items-center justify-between"
              >
                <div>
                  <i class="pi pi-exclamation-triangle mr-2"></i>
                  <strong>Vysoké využití kontextu:</strong> Aktuální kontext obsahuje {{ Math.round(contextStats.estimated_tokens / 1000) }}k tokenů.
                </div>
                <Button
                  label="Spravovat kontext"
                  icon="pi pi-sliders-h"
                  @click="showContextManager = true"
                  size="small"
                  severity="warning"
                />
              </div>
            </div>
          </div>

          <!-- Input -->
          <ChatInput
            :disabled="!chatStore.currentConversation"
            :is-streaming="chatStore.isStreaming"
            @send="handleSend"
            @stop="chatStore.stopGeneration"
          />
        </div>

        <!-- Debug Panel -->
        <DebugPanel
          v-if="showDebug"
          :metrics="chatStore.currentMetrics"
          :debug-info="chatStore.debugInfo"
          :total-tokens="chatStore.totalTokensUsed"
          :is-streaming="chatStore.isStreaming"
          :conversation="chatStore.currentConversation"
        />
      </div>
    </div>

    <!-- Settings Dialog -->
    <SettingsPanel
      v-model:visible="showSettings"
      :conversation="chatStore.currentConversation"
      @create="handleCreateConversation"
      @save="handleSaveConversation"
    />

    <!-- Context Manager Dialog -->
    <ContextManagerPanel
      v-model:visible="showContextManager"
      :conversation="chatStore.currentConversation"
      @contextCompacted="handleContextCompacted"
    />
  </div>
</template>
