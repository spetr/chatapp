<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'
import ProgressBar from 'primevue/progressbar'
import Popover from 'primevue/popover'

const props = defineProps<{
  conversationId: string | null
}>()

interface ContextStats {
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

const stats = ref<ContextStats | null>(null)
const isLoading = ref(false)
const detailsPanel = ref()

async function loadStats() {
  if (!props.conversationId) {
    stats.value = null
    return
  }

  isLoading.value = true
  try {
    const response = await fetch(`/api/conversations/${props.conversationId}/context-stats`)
    if (response.ok) {
      stats.value = await response.json()
    }
  } catch (error) {
    console.error('Failed to load context stats:', error)
  } finally {
    isLoading.value = false
  }
}

// Reload stats when conversation changes or periodically
watch(() => props.conversationId, loadStats, { immediate: true })

// Reload every 30 seconds while conversation is active
const interval = setInterval(() => {
  if (props.conversationId) {
    loadStats()
  }
}, 30000)

onUnmounted(() => {
  clearInterval(interval)
})

function getStatusColor(status: string): string {
  switch (status) {
    case 'critical': return 'bg-red-500'
    case 'warning': return 'bg-yellow-500'
    case 'info': return 'bg-blue-500'
    default: return 'bg-green-500'
  }
}

function getProgressColor(percent: number): string {
  if (percent > 90) return '#ef4444' // red
  if (percent > 70) return '#f59e0b' // yellow
  if (percent > 50) return '#3b82f6' // blue
  return '#22c55e' // green
}

function formatTokens(n: number): string {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
  return n.toString()
}

function toggleDetails(event: Event) {
  detailsPanel.value.toggle(event)
}
</script>

<template>
  <div v-if="stats" class="flex items-center gap-2">
    <!-- Compact status indicator -->
    <button
      @click="toggleDetails"
      class="flex items-center gap-2 px-2 py-1 rounded hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
      :class="{ 'animate-pulse': stats.status === 'critical' }"
    >
      <!-- Status dot -->
      <span
        class="w-2 h-2 rounded-full"
        :class="getStatusColor(stats.status)"
      />

      <!-- Token count -->
      <span class="text-xs font-mono">
        {{ formatTokens(stats.estimated_tokens) }}
      </span>

      <!-- Mini progress bar -->
      <div class="w-16 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
        <div
          class="h-full transition-all duration-300"
          :style="{
            width: `${Math.min(stats.token_percent_used, 100)}%`,
            backgroundColor: getProgressColor(stats.token_percent_used)
          }"
        />
      </div>

      <!-- Expand icon -->
      <i class="pi pi-chevron-down text-xs text-gray-400" />
    </button>

    <!-- Details panel -->
    <Popover ref="detailsPanel" :style="{ width: '20rem' }">
      <div class="space-y-4">
        <h3 class="font-semibold text-sm flex items-center gap-2">
          <i class="pi pi-database" />
          Stav kontextu
        </h3>

        <!-- Main progress -->
        <div>
          <div class="flex justify-between text-xs mb-1">
            <span>Využití tokenů</span>
            <span class="font-mono">
              {{ formatTokens(stats.estimated_tokens) }} / {{ formatTokens(stats.max_tokens) }}
            </span>
          </div>
          <ProgressBar
            :value="Math.min(stats.token_percent_used, 100)"
            :showValue="false"
            :style="{ height: '0.5rem' }"
            :pt="{
              value: { style: { backgroundColor: getProgressColor(stats.token_percent_used) } }
            }"
          />
          <div class="text-xs text-gray-500 mt-1">
            {{ stats.token_percent_used.toFixed(1) }}% kontextového okna využito
          </div>
        </div>

        <!-- Stats grid -->
        <div class="grid grid-cols-2 gap-2 text-xs">
          <div class="p-2 bg-gray-50 dark:bg-gray-800 rounded">
            <div class="text-gray-500">Zpráv</div>
            <div class="font-mono font-medium">{{ stats.message_count }}</div>
          </div>
          <div class="p-2 bg-gray-50 dark:bg-gray-800 rounded">
            <div class="text-gray-500">Odhad ceny (další zpr.)</div>
            <div class="font-mono font-medium">${{ stats.estimated_input_cost.toFixed(4) }}</div>
          </div>
        </div>

        <!-- Caching status -->
        <div
          v-if="stats.caching_enabled"
          class="flex items-center gap-2 p-2 bg-green-50 dark:bg-green-900/20 rounded text-xs"
        >
          <i class="pi pi-check-circle text-green-500" />
          <span class="text-green-700 dark:text-green-300">
            Prompt caching aktivní - úspora až 90% na opakovaném kontextu
          </span>
        </div>
        <div
          v-else
          class="flex items-center gap-2 p-2 bg-gray-50 dark:bg-gray-800 rounded text-xs"
        >
          <i class="pi pi-info-circle text-gray-400" />
          <span class="text-gray-600 dark:text-gray-400">
            Prompt caching není dostupný pro tohoto poskytovatele
          </span>
        </div>

        <!-- Recommendations -->
        <div v-if="stats.recommendations.length > 0" class="space-y-1">
          <div class="text-xs font-medium text-gray-500">Doporučení</div>
          <div
            v-for="(rec, index) in stats.recommendations"
            :key="index"
            class="flex items-start gap-2 text-xs p-2 rounded"
            :class="{
              'bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300': stats.status === 'critical',
              'bg-yellow-50 dark:bg-yellow-900/20 text-yellow-700 dark:text-yellow-300': stats.status === 'warning',
              'bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-300': stats.status === 'info',
            }"
          >
            <i class="pi pi-lightbulb mt-0.5" />
            <span>{{ rec }}</span>
          </div>
        </div>

        <!-- How it works -->
        <div class="border-t border-gray-200 dark:border-gray-700 pt-3">
          <details class="text-xs">
            <summary class="cursor-pointer text-gray-500 hover:text-gray-700 dark:hover:text-gray-300">
              Jak funguje správa kontextu
            </summary>
            <div class="mt-2 space-y-2 text-gray-600 dark:text-gray-400">
              <p>
                <strong>Plná historie:</strong> Každá zpráva, kterou odešlete, obsahuje celou historii konverzace.
                To znamená, že delší konverzace stojí více tokenů.
              </p>
              <p>
                <strong>Prompt Caching (Claude):</strong> Anthropic cachuje části vaší konverzace.
                Když je stejný kontext odeslán znovu, platíte pouze 10% normální ceny.
              </p>
              <p>
                <strong>Chytrá zkrácení:</strong> Při blížení se limitům mohou být starší zprávy shrnuty,
                aby se snížily náklady při zachování důležitého kontextu.
              </p>
            </div>
          </details>
        </div>
      </div>
    </Popover>
  </div>
</template>
