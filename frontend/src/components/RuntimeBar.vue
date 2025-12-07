<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import type { Conversation, Metrics, DebugInfo, MCPStatus } from '@/types'
import { getMCPStatus } from '@/api/client'
import Badge from 'primevue/badge'
import ContextStatus from '@/components/ContextStatus.vue'

const props = defineProps<{
  conversation: Conversation | null
  metrics: Metrics | null
  debugInfo: DebugInfo | null
  isStreaming: boolean
  totalTokens: number
}>()

// MCP Status
const mcpStatus = ref<MCPStatus | null>(null)

async function loadMCPStatus() {
  try {
    mcpStatus.value = await getMCPStatus()
  } catch (error) {
    console.error('Failed to load MCP status:', error)
  }
}

onMounted(() => {
  loadMCPStatus()
})

// Refresh MCP status periodically
const mcpInterval = setInterval(loadMCPStatus, 60000)
onUnmounted(() => clearInterval(mcpInterval))

// Computed values
const streamingEnabled = computed(() => {
  return props.conversation?.settings?.stream !== false
})

const thinkingEnabled = computed(() => {
  return props.conversation?.settings?.enable_thinking === true
})

const thinkingBudget = computed(() => {
  if (!thinkingEnabled.value) return null
  const budget = props.conversation?.settings?.thinking_budget
  if (!budget) return null
  return { low: 'Nízká', medium: 'Střední', high: 'Vysoká' }[budget] || budget
})

const toolsEnabled = computed(() => {
  return props.conversation?.settings?.enable_tools === true
})

const formattedTTFB = computed(() => {
  if (!props.metrics?.ttfb_ms) return null
  const ms = props.metrics.ttfb_ms
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
})

const formattedLatency = computed(() => {
  if (!props.metrics?.total_latency_ms) return null
  const ms = props.metrics.total_latency_ms
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
})

const formattedTPS = computed(() => {
  if (!props.metrics?.tokens_per_second) return null
  return `${Math.round(props.metrics.tokens_per_second)} tok/s`
})

const formattedTokens = computed(() => {
  if (!props.metrics) return null
  return `${props.metrics.input_tokens} → ${props.metrics.output_tokens}`
})

const cacheInfo = computed(() => {
  if (!props.metrics) return null
  const created = props.metrics.cache_creation_input_tokens
  const read = props.metrics.cache_read_input_tokens
  if (!created && !read) return null
  return { created: created || 0, read: read || 0 }
})

const formatNumber = (n: number): string => {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
  return n.toString()
}
</script>

<template>
  <div
    v-if="conversation"
    class="border-b border-gray-200 dark:border-gray-700 bg-gradient-to-r from-gray-50 to-white dark:from-gray-900 dark:to-gray-800"
  >
    <!-- Main Runtime Bar -->
    <div class="px-4 py-2 flex items-center gap-4 flex-wrap">
      <!-- Provider/Model -->
      <div class="flex items-center gap-1.5 text-xs text-gray-500">
        <span class="px-2 py-0.5 bg-gray-100 dark:bg-gray-800 rounded font-medium">
          {{ conversation.provider }}
        </span>
        <span class="text-gray-400">/</span>
        <span class="font-mono truncate max-w-[12rem]">{{ conversation.model }}</span>
      </div>

      <!-- Context Status -->
      <ContextStatus :conversation-id="conversation?.id || null" />

      <!-- Divider -->
      <div class="w-px h-6 bg-gray-300 dark:bg-gray-600" />

      <!-- Feature Toggles -->
      <div class="flex items-center gap-2">
        <!-- Streaming -->
        <div
          class="flex items-center gap-1 px-2 py-1 rounded-full text-xs font-medium"
          :class="streamingEnabled
            ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
            : 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-500'"
        >
          <i :class="streamingEnabled ? 'pi pi-bolt' : 'pi pi-minus'" class="text-2xs" />
          Stream
        </div>

        <!-- Thinking -->
        <div
          class="flex items-center gap-1 px-2 py-1 rounded-full text-xs font-medium"
          :class="thinkingEnabled
            ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
            : 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-500'"
        >
          <i :class="thinkingEnabled ? 'pi pi-lightbulb' : 'pi pi-minus'" class="text-2xs" />
          Think
          <span v-if="thinkingBudget" class="opacity-75">({{ thinkingBudget }})</span>
        </div>

        <!-- Tools/MCP -->
        <div
          class="flex items-center gap-1 px-2 py-1 rounded-full text-xs font-medium"
          :class="toolsEnabled && mcpStatus?.enabled
            ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
            : 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-500'"
        >
          <i :class="toolsEnabled ? 'pi pi-wrench' : 'pi pi-minus'" class="text-2xs" />
          MCP
          <Badge
            v-if="mcpStatus?.enabled && mcpStatus.total_tools > 0"
            :value="mcpStatus.total_tools"
            severity="info"
            class="scale-75"
          />
        </div>
      </div>

      <!-- Divider -->
      <div class="w-px h-6 bg-gray-300 dark:bg-gray-600" />

      <!-- Live Metrics -->
      <div class="flex items-center gap-3 text-xs">
        <!-- Streaming indicator -->
        <div v-if="isStreaming" class="flex items-center gap-1 text-green-600 dark:text-green-400">
          <span class="relative flex h-2 w-2">
            <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
            <span class="relative inline-flex rounded-full h-2 w-2 bg-green-500"></span>
          </span>
          <span class="font-medium">Generuji...</span>
        </div>

        <!-- Metrics display -->
        <template v-if="metrics && !isStreaming">
          <!-- TTFB -->
          <div class="flex items-center gap-1 text-gray-600 dark:text-gray-400" v-tooltip="'Time to First Byte'">
            <i class="pi pi-clock text-2xs" />
            <span class="font-mono">{{ formattedTTFB }}</span>
          </div>

          <!-- Total Latency -->
          <div class="flex items-center gap-1 text-gray-600 dark:text-gray-400" v-tooltip="'Celková latence'">
            <i class="pi pi-stopwatch text-2xs" />
            <span class="font-mono">{{ formattedLatency }}</span>
          </div>

          <!-- Tokens per second -->
          <div class="flex items-center gap-1 text-gray-600 dark:text-gray-400" v-tooltip="'Rychlost generování'">
            <i class="pi pi-bolt text-2xs" />
            <span class="font-mono">{{ formattedTPS }}</span>
          </div>

          <!-- Token counts -->
          <div class="flex items-center gap-1 text-gray-600 dark:text-gray-400" v-tooltip="'Input → Output tokeny'">
            <i class="pi pi-sort-alt text-2xs" />
            <span class="font-mono">{{ formattedTokens }}</span>
          </div>

          <!-- Cache info (if available) -->
          <div
            v-if="cacheInfo"
            class="flex items-center gap-1 text-blue-600 dark:text-blue-400"
            v-tooltip="'Prompt Cache: vytvořeno / čteno'"
          >
            <i class="pi pi-database text-2xs" />
            <span class="font-mono">{{ formatNumber(cacheInfo.created) }}/{{ formatNumber(cacheInfo.read) }}</span>
          </div>
        </template>
      </div>

      <!-- Spacer -->
      <div class="flex-1" />

      <!-- Total session tokens -->
      <div
        class="flex items-center gap-2 px-3 py-1 bg-gray-100 dark:bg-gray-800 rounded-full text-xs"
        v-tooltip="'Celkový počet tokenů v této konverzaci'"
      >
        <i class="pi pi-chart-bar text-gray-500" />
        <span class="font-mono font-medium">{{ formatNumber(totalTokens) }}</span>
        <span class="text-gray-500">celkem</span>
      </div>
    </div>

    <!-- Debug request info (compact, when available) -->
    <div
      v-if="debugInfo?.request"
      class="px-4 py-1 bg-gray-100 dark:bg-gray-800/50 border-t border-gray-200 dark:border-gray-700 text-xs font-mono text-gray-500 dark:text-gray-400 flex items-center gap-4"
    >
      <span>
        <span class="text-gray-400">API:</span>
        {{ debugInfo.request.method }} {{ debugInfo.request.url }}
      </span>
      <span v-if="debugInfo.request.thinking_enabled" class="text-purple-500">
        <i class="pi pi-lightbulb" /> thinking
      </span>
      <span v-if="debugInfo.request.body?.tools?.length" class="text-blue-500">
        <i class="pi pi-wrench" /> {{ debugInfo.request.body.tools.length }} tools
      </span>
    </div>
  </div>
</template>
