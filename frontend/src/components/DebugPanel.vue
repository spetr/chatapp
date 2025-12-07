<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useChatStore } from '@/stores/chat'
import type { Metrics, DebugInfo, Conversation, MCPStatus } from '@/types'
import Accordion from 'primevue/accordion'
import AccordionPanel from 'primevue/accordionpanel'
import AccordionHeader from 'primevue/accordionheader'
import AccordionContent from 'primevue/accordioncontent'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import * as api from '@/api/client'
import type { MCPTool } from '@/types'

interface ModelCapabilities {
  name: string
  family?: string
  parameterSize?: string
  supportsThinking?: boolean
  supportsTools?: boolean
  contextWindow?: string
}

const props = defineProps<{
  metrics: Metrics | null
  debugInfo: DebugInfo | null
  totalTokens: number
  isStreaming: boolean
  conversation?: Conversation | null
  modelCapabilities?: ModelCapabilities | null
}>()

const chatStore = useChatStore()

// Resizable panel state
const panelWidth = ref(320)
const isResizing = ref(false)
const minWidth = 240
const maxWidth = 600

// MCP status
const mcpStatus = ref<MCPStatus | null>(null)
const mcpLoading = ref(false)

// Tool details modal
const showToolModal = ref(false)
const selectedTool = ref<MCPTool | null>(null)
const selectedServerName = ref('')

function openToolDetails(tool: MCPTool, serverName: string) {
  selectedTool.value = tool
  selectedServerName.value = serverName
  showToolModal.value = true
}

// Accordion state - multiple sections can be open
const expandedSections = ref<string[]>(['metrics'])

const formattedRequest = computed(() => {
  if (!props.debugInfo?.request) return null
  return JSON.stringify(props.debugInfo.request, null, 2)
})

const formattedResponse = computed(() => {
  if (!props.debugInfo?.response) return null
  return JSON.stringify(props.debugInfo.response, null, 2)
})

// Pricing for local providers (Ollama, llama.cpp) - fetched separately
const localPricing = ref<{ input_per_1m: number; output_per_1m: number } | null>(null)

// Get pricing for the current model
const currentModelPricing = computed(() => {
  if (!props.conversation?.model) return null

  // For local providers, use fetched pricing
  const provider = props.conversation.provider
  if (provider === 'ollama' || provider === 'llamacpp') {
    return localPricing.value
  }

  // For cloud providers, get from models list
  const model = chatStore.models.find(m => m.id === props.conversation!.model)
  return model?.pricing ?? null
})

// Fetch pricing for local providers
async function fetchLocalPricing() {
  if (!props.conversation?.provider) return
  const provider = props.conversation.provider
  if (provider !== 'ollama' && provider !== 'llamacpp') return

  try {
    const response = await fetch(`/api/pricing?provider=${provider}&model=${props.conversation.model}`)
    if (response.ok) {
      const data = await response.json()
      localPricing.value = {
        input_per_1m: data.input_per_1m,
        output_per_1m: data.output_per_1m,
      }
    }
  } catch (error) {
    console.error('Failed to fetch local pricing:', error)
  }
}

// Watch for conversation changes to refetch pricing
watch(() => props.conversation?.id, () => {
  if (props.conversation?.provider === 'ollama' || props.conversation?.provider === 'llamacpp') {
    fetchLocalPricing()
  } else {
    localPricing.value = null
  }
}, { immediate: true })

const costEstimate = computed(() => {
  if (!props.metrics) return null
  const pricing = currentModelPricing.value
  if (!pricing) return null

  const inputCost = (props.metrics.input_tokens / 1_000_000) * pricing.input_per_1m
  const outputCost = (props.metrics.output_tokens / 1_000_000) * pricing.output_per_1m
  return {
    input: inputCost.toFixed(6),
    output: outputCost.toFixed(6),
    total: (inputCost + outputCost).toFixed(6),
  }
})

function copyToClipboard(text: string) {
  navigator.clipboard.writeText(text)
}

function fmt(n: number): string {
  return n.toLocaleString()
}

// Resize handlers
function startResize(event: MouseEvent) {
  isResizing.value = true
  event.preventDefault()
  document.addEventListener('mousemove', handleResize)
  document.addEventListener('mouseup', stopResize)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}

function handleResize(event: MouseEvent) {
  if (!isResizing.value) return
  const newWidth = window.innerWidth - event.clientX
  if (newWidth >= minWidth && newWidth <= maxWidth) {
    panelWidth.value = newWidth
  }
}

function stopResize() {
  isResizing.value = false
  document.removeEventListener('mousemove', handleResize)
  document.removeEventListener('mouseup', stopResize)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
}

async function loadMCPStatus() {
  mcpLoading.value = true
  try {
    mcpStatus.value = await api.getMCPStatus()
  } catch (error) {
    console.error('Failed to load MCP status:', error)
  } finally {
    mcpLoading.value = false
  }
}

onMounted(() => {
  loadMCPStatus()
})

onUnmounted(() => {
  document.removeEventListener('mousemove', handleResize)
  document.removeEventListener('mouseup', stopResize)
})
</script>

<template>
  <div
    class="debug-panel border-l border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 overflow-hidden flex flex-col relative"
    :style="{ width: `${panelWidth}px` }"
  >
    <!-- Resize handle -->
    <div
      class="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-blue-500/50 z-10 transition-colors"
      :class="{ 'bg-blue-500/50': isResizing }"
      @mousedown="startResize"
    />

    <!-- Header -->
    <div class="px-3 py-2 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 flex items-center justify-between">
      <span class="font-medium text-sm flex items-center gap-2">
        <i class="pi pi-sliders-h text-xs"></i>
        Ladění
      </span>
      <div class="flex items-center gap-1">
        <span v-if="isStreaming" class="flex items-center gap-1 text-xs text-blue-500">
          <i class="pi pi-spin pi-spinner text-xs"></i>
        </span>
      </div>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      <!-- Educational info -->
      <div class="p-3 text-xs text-gray-600 dark:text-gray-400 bg-blue-50 dark:bg-blue-900/20 border-b border-blue-200 dark:border-blue-800">
        <p class="font-medium text-blue-700 dark:text-blue-300 mb-1">
          <i class="pi pi-info-circle mr-1"></i>
          Ladící panel
        </p>
        <p class="text-2xs">
          Zde vidíte detailní informace o komunikaci s LLM modelem - tokeny, rychlost, cenu a surová JSON data požadavků a odpovědí.
        </p>
      </div>
      <Accordion v-model:value="expandedSections" multiple class="debug-accordion">

        <!-- Metrics Section -->
        <AccordionPanel value="metrics">
          <AccordionHeader>
            <div class="flex items-center gap-2 text-sm">
              <i class="pi pi-chart-bar text-blue-500"></i>
              <span>Metriky</span>
              <span v-if="metrics" class="ml-auto text-xs text-gray-400 font-mono">
                {{ fmt(metrics.total_tokens) }} tok
              </span>
            </div>
          </AccordionHeader>
          <AccordionContent>
            <div class="space-y-2 text-xs">
              <template v-if="metrics">
                <!-- Token counts -->
                <div class="grid grid-cols-2 gap-1.5">
                  <div class="stat-box cursor-help" v-tooltip="'Vstupní tokeny: systémový prompt + historie konverzace + váš dotaz. Více tokenů = vyšší cena.'">
                    <span class="stat-label">Vstup</span>
                    <span class="stat-value">{{ fmt(metrics.input_tokens) }}</span>
                  </div>
                  <div class="stat-box cursor-help" v-tooltip="'Výstupní tokeny: počet tokenů v odpovědi modelu. Generování výstupu je obvykle dražší než vstup.'">
                    <span class="stat-label">Výstup</span>
                    <span class="stat-value">{{ fmt(metrics.output_tokens) }}</span>
                  </div>
                </div>

                <!-- Performance -->
                <div class="grid grid-cols-2 gap-1.5">
                  <div class="stat-box cursor-help" v-tooltip="'Time To First Byte: doba od odeslání požadavku do přijetí první odpovědi. Ukazuje latenci serveru a modelu.'">
                    <span class="stat-label">TTFB</span>
                    <span class="stat-value">{{ metrics.ttfb_ms.toFixed(0) }}<small>ms</small></span>
                  </div>
                  <div class="stat-box cursor-help" v-tooltip="'Rychlost generování: kolik tokenů model generuje za sekundu. Vyšší = rychlejší odpovědi.'">
                    <span class="stat-label">Rychlost</span>
                    <span class="stat-value">{{ metrics.tokens_per_second.toFixed(0) }}<small>t/s</small></span>
                  </div>
                </div>

                <!-- Cache -->
                <div v-if="metrics.cache_read_input_tokens" class="p-2 bg-green-500/10 rounded border border-green-500/20 cursor-help" v-tooltip="'Prompt caching: část vstupních tokenů byla načtena z cache, což snižuje náklady a latenci. Funguje u Claude a některých dalších poskytovatelů.'">
                  <div class="flex justify-between items-center">
                    <span class="text-green-600 dark:text-green-400">Cache hit</span>
                    <span class="font-mono">{{ fmt(metrics.cache_read_input_tokens) }}</span>
                  </div>
                </div>

                <!-- Cost estimate -->
                <div v-if="costEstimate" class="p-2 bg-yellow-500/10 rounded border border-yellow-500/20 cursor-help" v-tooltip="'Odhadovaná cena této zprávy na základě počtu tokenů a ceníku poskytovatele. Skutečná cena se může mírně lišit.'">
                  <div class="flex justify-between">
                    <span class="text-yellow-600 dark:text-yellow-400">Cena</span>
                    <span class="font-mono">${{ costEstimate.total }}</span>
                  </div>
                </div>

                <!-- Session total -->
                <div class="pt-2 border-t border-gray-200 dark:border-gray-700">
                  <div class="flex justify-between text-gray-500">
                    <span>Celkem v relaci</span>
                    <span class="font-mono">{{ fmt(totalTokens) }} tokenů</span>
                  </div>
                </div>
              </template>

              <div v-else class="text-center py-4 text-gray-400">
                <i class="pi pi-chart-line text-2xl mb-1"></i>
                <p>Žádná data</p>
              </div>
            </div>
          </AccordionContent>
        </AccordionPanel>

        <!-- Model Section -->
        <AccordionPanel value="model">
          <AccordionHeader>
            <div class="flex items-center gap-2 text-sm">
              <i class="pi pi-box text-purple-500"></i>
              <span>Model</span>
              <span v-if="conversation" class="ml-auto text-xs text-gray-400 truncate max-w-[6.25rem]">
                {{ conversation.model.split('/').pop() }}
              </span>
            </div>
          </AccordionHeader>
          <AccordionContent>
            <div class="space-y-2 text-xs">
              <template v-if="conversation">
                <div class="info-row">
                  <span class="info-label">Poskytovatel</span>
                  <span class="info-value capitalize">{{ conversation.provider }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">Model</span>
                  <span class="info-value font-mono text-2xs break-all">{{ conversation.model }}</span>
                </div>

                <!-- Feature flags - always show -->
                <div class="pt-2 border-t border-gray-200 dark:border-gray-700 space-y-1">
                  <div class="info-row cursor-help" v-tooltip="'Streaming: odpověď se zobrazuje postupně, jak ji model generuje. Rychlejší odezva, ale některé funkce nemusí být dostupné.'">
                    <span class="info-label">Streaming</span>
                    <span class="info-value">
                      <i :class="conversation.settings?.stream !== false ? 'pi pi-check text-green-500' : 'pi pi-times text-gray-400'"></i>
                    </span>
                  </div>
                  <div class="info-row cursor-help" v-tooltip="'Extended Thinking: model přemýšlí před odpovědí. Zlepšuje kvalitu u složitých úloh, ale zvyšuje latenci a spotřebu tokenů.'">
                    <span class="info-label">Thinking</span>
                    <span class="info-value">
                      <i :class="conversation.settings?.enable_thinking ? 'pi pi-check text-purple-500' : 'pi pi-times text-gray-400'"></i>
                    </span>
                  </div>
                  <div class="info-row cursor-help" v-tooltip="'Tool Use: model může volat externí nástroje (MCP) pro získání informací nebo provedení akcí. Implementace ReAct patternu.'">
                    <span class="info-label">Tools</span>
                    <span class="info-value">
                      <i :class="conversation.settings?.enable_tools ? 'pi pi-check text-blue-500' : 'pi pi-times text-gray-400'"></i>
                    </span>
                  </div>
                </div>

                <!-- Other settings if available -->
                <div v-if="conversation.settings?.temperature != null || conversation.settings?.max_tokens" class="pt-2 border-t border-gray-200 dark:border-gray-700 space-y-1">
                  <div v-if="conversation.settings.temperature != null" class="info-row">
                    <span class="info-label">Teplota</span>
                    <span class="info-value font-mono">{{ conversation.settings.temperature }}</span>
                  </div>
                  <div v-if="conversation.settings.max_tokens" class="info-row">
                    <span class="info-label">Max tokenů</span>
                    <span class="info-value font-mono">{{ conversation.settings.max_tokens }}</span>
                  </div>
                </div>

                <!-- System prompt preview -->
                <div v-if="conversation.system_prompt" class="pt-2 border-t border-gray-200 dark:border-gray-700">
                  <div class="info-label mb-1">Systémový prompt</div>
                  <div class="p-1.5 bg-gray-100 dark:bg-gray-800 rounded text-2xs text-gray-600 dark:text-gray-400 max-h-16 overflow-y-auto">
                    {{ conversation.system_prompt.substring(0, 150) }}{{ conversation.system_prompt.length > 150 ? '...' : '' }}
                  </div>
                </div>
              </template>

              <div v-else class="text-center py-4 text-gray-400">
                <i class="pi pi-box text-2xl mb-1"></i>
                <p>Vyberte konverzaci</p>
              </div>
            </div>
          </AccordionContent>
        </AccordionPanel>

        <!-- MCP Section -->
        <AccordionPanel value="mcp">
          <AccordionHeader>
            <div class="flex items-center gap-2 text-sm">
              <i class="pi pi-sitemap text-green-500"></i>
              <span>MCP</span>
              <span v-if="mcpStatus" class="ml-auto flex items-center gap-1">
                <span
                  class="w-2 h-2 rounded-full"
                  :class="mcpStatus.enabled ? 'bg-green-500' : 'bg-gray-400'"
                ></span>
                <span class="text-xs text-gray-400">{{ mcpStatus.total_tools }}</span>
              </span>
            </div>
          </AccordionHeader>
          <AccordionContent>
            <div class="space-y-2 text-xs">
              <!-- Refresh button -->
              <div class="flex justify-end">
                <Button
                  icon="pi pi-refresh"
                  size="small"
                  text
                  rounded
                  @click="loadMCPStatus"
                  :loading="mcpLoading"
                  class="!p-1"
                  v-tooltip="'Znovu načíst stav MCP serverů a jejich nástrojů'"
                />
              </div>

              <template v-if="mcpStatus">
                <!-- Summary -->
                <div class="grid grid-cols-2 gap-1.5">
                  <div class="stat-box cursor-help" v-tooltip="'Počet připojených MCP serverů. Každý server může poskytovat více nástrojů.'">
                    <span class="stat-label">Servery</span>
                    <span class="stat-value">{{ mcpStatus.server_count }}</span>
                  </div>
                  <div class="stat-box cursor-help" v-tooltip="'Celkový počet dostupných nástrojů ze všech MCP serverů. Model může tyto nástroje volat během konverzace.'">
                    <span class="stat-label">Nástroje</span>
                    <span class="stat-value">{{ mcpStatus.total_tools }}</span>
                  </div>
                </div>

                <!-- Status -->
                <div
                  class="p-2 rounded flex items-center gap-2 cursor-help"
                  :class="mcpStatus.enabled ? 'bg-green-500/10 border border-green-500/20' : 'bg-gray-100 dark:bg-gray-800'"
                  v-tooltip="mcpStatus.enabled ? 'MCP je aktivní - model může používat externí nástroje' : 'MCP není aktivní - zkontrolujte konfiguraci serverů'"
                >
                  <i
                    class="pi text-sm"
                    :class="mcpStatus.enabled ? 'pi-check-circle text-green-500' : 'pi-times-circle text-gray-400'"
                  ></i>
                  <span :class="mcpStatus.enabled ? 'text-green-600 dark:text-green-400' : 'text-gray-500'">
                    {{ mcpStatus.enabled ? 'Aktivní' : 'Neaktivní' }}
                  </span>
                </div>

                <!-- Servers -->
                <div v-if="mcpStatus.servers.length > 0" class="space-y-1.5">
                  <div
                    v-for="server in mcpStatus.servers"
                    :key="server.name"
                    class="p-2 bg-white dark:bg-gray-800 rounded border border-gray-200 dark:border-gray-700"
                  >
                    <div class="flex items-center justify-between mb-1">
                      <span class="font-medium truncate">{{ server.name }}</span>
                      <span
                        class="w-2 h-2 rounded-full flex-shrink-0"
                        :class="server.connected ? 'bg-green-500' : 'bg-red-500'"
                      ></span>
                    </div>
                    <div class="text-2xs text-gray-500 font-mono truncate">
                      {{ server.command }}
                    </div>
                    <div class="mt-1 text-gray-400">
                      {{ server.tool_count }} nástrojů
                    </div>

                    <!-- Tools -->
                    <details v-if="server.tools.length" class="mt-1">
                      <summary class="cursor-pointer text-gray-500 hover:text-gray-700 dark:hover:text-gray-300 text-2xs">
                        Zobrazit nástroje ({{ server.tools.length }})
                      </summary>
                      <div class="mt-1 space-y-0.5 max-h-32 overflow-y-auto">
                        <button
                          v-for="tool in server.tools"
                          :key="tool.name"
                          @click="openToolDetails(tool, server.name)"
                          class="w-full p-1.5 bg-gray-50 dark:bg-gray-900 rounded text-2xs text-left hover:bg-blue-50 dark:hover:bg-blue-900/30 transition-colors group"
                        >
                          <div class="flex items-center justify-between">
                            <span class="font-mono font-medium text-gray-800 dark:text-gray-200">{{ tool.name }}</span>
                            <i class="pi pi-external-link text-2xs text-gray-400 group-hover:text-blue-500 opacity-0 group-hover:opacity-100 transition-opacity"></i>
                          </div>
                          <p v-if="tool.description" class="text-gray-500 truncate mt-0.5">{{ tool.description }}</p>
                        </button>
                      </div>
                    </details>
                  </div>
                </div>

                <div v-else class="text-center py-2 text-gray-400">
                  Žádné servery
                </div>
              </template>

              <div v-else-if="mcpLoading" class="text-center py-4 text-gray-400">
                <i class="pi pi-spin pi-spinner"></i>
              </div>
            </div>
          </AccordionContent>
        </AccordionPanel>

        <!-- Request Section -->
        <AccordionPanel value="request">
          <AccordionHeader>
            <div class="flex items-center gap-2 text-sm">
              <i class="pi pi-arrow-up text-orange-500"></i>
              <span>Požadavek</span>
            </div>
          </AccordionHeader>
          <AccordionContent>
            <div class="text-xs">
              <template v-if="debugInfo?.request">
                <div class="flex justify-end mb-1">
                  <Button
                    icon="pi pi-copy"
                    size="small"
                    text
                    rounded
                    @click="copyToClipboard(formattedRequest || '')"
                    class="!p-1"
                    v-tooltip="'Kopírovat'"
                  />
                </div>

                <!-- Quick info -->
                <div class="space-y-1 mb-2">
                  <div v-if="debugInfo.request.url" class="info-row">
                    <span class="info-label">URL</span>
                    <span class="info-value font-mono text-2xs truncate" :title="debugInfo.request.url">
                      {{ debugInfo.request.url.split('/').slice(-2).join('/') }}
                    </span>
                  </div>
                  <div v-if="debugInfo.request.body?.messages" class="info-row">
                    <span class="info-label">Zpráv</span>
                    <span class="info-value font-mono">{{ debugInfo.request.body.messages.length }}</span>
                  </div>
                  <div v-if="debugInfo.request.thinking_enabled !== undefined" class="info-row">
                    <span class="info-label">Thinking (req)</span>
                    <span class="info-value">
                      <i :class="debugInfo.request.thinking_enabled ? 'pi pi-check text-purple-500' : 'pi pi-times text-gray-400'"></i>
                    </span>
                  </div>
                </div>

                <!-- Full JSON -->
                <pre class="p-2 bg-gray-900 text-gray-100 rounded text-2xs max-h-48 overflow-auto leading-tight whitespace-pre-wrap break-all w-full">{{ formattedRequest }}</pre>
              </template>

              <div v-else class="text-center py-4 text-gray-400">
                <i class="pi pi-arrow-up text-2xl mb-1"></i>
                <p>Žádný požadavek</p>
              </div>
            </div>
          </AccordionContent>
        </AccordionPanel>

        <!-- Response Section -->
        <AccordionPanel value="response">
          <AccordionHeader>
            <div class="flex items-center gap-2 text-sm">
              <i class="pi pi-arrow-down text-cyan-500"></i>
              <span>Odpověď</span>
            </div>
          </AccordionHeader>
          <AccordionContent>
            <div class="text-xs">
              <template v-if="debugInfo?.response">
                <div class="flex justify-end mb-1">
                  <Button
                    icon="pi pi-copy"
                    size="small"
                    text
                    rounded
                    @click="copyToClipboard(formattedResponse || '')"
                    class="!p-1"
                    v-tooltip="'Kopírovat'"
                  />
                </div>

                <!-- Full JSON -->
                <pre class="p-2 bg-gray-900 text-gray-100 rounded text-2xs max-h-48 overflow-auto leading-tight whitespace-pre-wrap break-all w-full">{{ formattedResponse }}</pre>
              </template>

              <div v-else class="text-center py-4 text-gray-400">
                <i class="pi pi-arrow-down text-2xl mb-1"></i>
                <p>Žádná odpověď</p>
              </div>
            </div>
          </AccordionContent>
        </AccordionPanel>

      </Accordion>
    </div>

    <!-- Streaming indicator -->
    <div
      v-if="isStreaming"
      class="px-3 py-2 border-t border-gray-200 dark:border-gray-700 bg-blue-500/10"
    >
      <div class="flex items-center gap-2 text-xs text-blue-600 dark:text-blue-400">
        <i class="pi pi-spin pi-spinner"></i>
        <span>Streamování...</span>
      </div>
    </div>

    <!-- Tool Details Modal -->
    <Dialog
      v-model:visible="showToolModal"
      modal
      :style="{ width: '37.5rem', maxWidth: '95vw' }"
      class="tool-details-dialog"
    >
      <template #header>
        <div class="modal-header">
          <div class="modal-header-icon">
            <i class="pi pi-wrench text-white"></i>
          </div>
          <div class="modal-header-text">
            <h2>{{ selectedTool?.name || 'Detail nástroje' }}</h2>
            <p>MCP nástroj ze serveru {{ selectedServerName }}</p>
          </div>
        </div>
      </template>
      <div v-if="selectedTool" class="space-y-4 max-h-[31rem] overflow-y-auto pr-2">
        <!-- Description -->
        <div v-if="selectedTool.description" class="space-y-1">
          <h4 class="section-title flex items-center gap-2">
            <i class="pi pi-info-circle icon-primary"></i>
            Popis
          </h4>
          <p class="text-sm text-gray-800 dark:text-gray-200 bg-gray-50 dark:bg-gray-800 p-3 rounded-lg">
            {{ selectedTool.description }}
          </p>
        </div>

        <!-- Input Schema -->
        <div v-if="selectedTool.inputSchema" class="space-y-1">
          <h4 class="section-title flex items-center gap-2">
            <i class="pi pi-code icon-purple"></i>
            Vstupní schéma
          </h4>
          <div class="bg-gray-900 rounded-lg p-3 overflow-auto max-h-64">
            <pre class="text-xs text-gray-100 font-mono whitespace-pre-wrap">{{ JSON.stringify(selectedTool.inputSchema, null, 2) }}</pre>
          </div>
        </div>

        <!-- Properties breakdown if available -->
        <div v-if="selectedTool.inputSchema?.properties" class="space-y-2">
          <h4 class="section-title flex items-center gap-2">
            <i class="pi pi-list icon-warning"></i>
            Parametry
          </h4>
          <div class="space-y-2">
            <div
              v-for="(prop, name) in (selectedTool.inputSchema.properties as Record<string, { type?: string; description?: string }>)"
              :key="String(name)"
              class="p-2 bg-gray-50 dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700"
            >
              <div class="flex items-center gap-2">
                <span class="font-mono font-medium text-sm text-gray-800 dark:text-gray-200">{{ name }}</span>
                <span v-if="prop.type" class="badge badge-blue font-mono">
                  {{ prop.type }}
                </span>
                <span
                  v-if="(selectedTool.inputSchema?.required as string[] | undefined)?.includes(String(name))"
                  class="badge badge-red"
                >
                  povinný
                </span>
              </div>
              <p v-if="prop.description" class="text-xs text-gray-500 mt-1">{{ prop.description }}</p>
            </div>
          </div>
        </div>

        <!-- Copy button -->
        <div v-if="selectedTool?.inputSchema" class="flex justify-end pt-2 border-t border-gray-200 dark:border-gray-700">
          <Button
            label="Kopírovat schéma"
            icon="pi pi-copy"
            size="small"
            severity="secondary"
            @click="copyToClipboard(JSON.stringify(selectedTool?.inputSchema, null, 2))"
            v-tooltip="'Kopírovat JSON schéma do schránky pro použití v kódu nebo dokumentaci'"
          />
        </div>
      </div>
    </Dialog>
  </div>
</template>

<style scoped>
.debug-panel {
  font-size: 0.8125rem; /* 13px */
}

.debug-accordion :deep(.p-accordionheader) {
  @apply px-3 py-2 bg-white dark:bg-gray-800 border-b border-gray-100 dark:border-gray-700;
}

.debug-accordion :deep(.p-accordionheader:hover) {
  @apply bg-gray-50 dark:bg-gray-700;
}

.debug-accordion :deep(.p-accordioncontent-content) {
  @apply px-3 py-2 bg-gray-50 dark:bg-gray-900;
}

.debug-accordion :deep(.p-accordionpanel) {
  @apply border-b border-gray-100 dark:border-gray-800;
}

.stat-box {
  @apply p-2 bg-white dark:bg-gray-800 rounded border border-gray-100 dark:border-gray-700 flex flex-col;
}

.stat-label {
  font-size: 0.625rem; /* 10px */
  @apply text-gray-500 uppercase tracking-wide;
}

.stat-value {
  @apply font-mono font-semibold text-sm;
}

.stat-value small {
  font-size: 0.625rem; /* 10px */
  @apply text-gray-400 ml-0.5;
}

.info-row {
  @apply flex justify-between items-center gap-2;
}

.info-label {
  @apply text-gray-500 flex-shrink-0;
}

.info-value {
  @apply text-right text-gray-900 dark:text-gray-100;
}
</style>
