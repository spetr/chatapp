<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js'
import katex from 'katex'
import 'katex/dist/katex.min.css'
import type { Message, ToolCall } from '@/types'
import Button from 'primevue/button'

const props = defineProps<{
  message: Message
  isStreaming?: boolean
  streamingToolCalls?: ToolCall[]
  currentIteration?: number
  maxIterations?: number
}>()

// State for tool calls panel
const showToolCalls = ref(false)
// Track which individual tool calls are expanded (by ID)
const expandedTools = ref<Set<string>>(new Set())

// Get tool calls - either from message or streaming
const toolCalls = computed(() => {
  if (props.streamingToolCalls && props.streamingToolCalls.length > 0) {
    return props.streamingToolCalls
  }
  return props.message.tool_calls || []
})

const hasToolCalls = computed(() => toolCalls.value.length > 0)

// Get unique iterations from tool calls
const uniqueIterations = computed(() => {
  const iterations = new Set<number>()
  for (const tc of toolCalls.value) {
    iterations.add(tc.iteration || 1)
  }
  return Array.from(iterations).sort((a, b) => a - b)
})

// Check if we have multiple iterations (ReAct loop)
const hasMultipleIterations = computed(() => uniqueIterations.value.length > 1)

// Group tool calls by iteration
const toolCallsByIteration = computed(() => {
  const groups: Record<number, ToolCall[]> = {}
  for (const tc of toolCalls.value) {
    const iter = tc.iteration || 1
    if (!groups[iter]) {
      groups[iter] = []
    }
    groups[iter].push(tc)
  }
  return groups
})

// Auto-expand tool calls when streaming
watch([() => props.isStreaming, hasToolCalls], ([streaming, hasTc]) => {
  if (streaming && hasTc) {
    showToolCalls.value = true
  }
}, { immediate: true })

// Toggle individual tool expansion
function toggleToolExpanded(toolId: string) {
  if (expandedTools.value.has(toolId)) {
    expandedTools.value.delete(toolId)
  } else {
    expandedTools.value.add(toolId)
  }
  // Force reactivity
  expandedTools.value = new Set(expandedTools.value)
}

function isToolExpanded(toolId: string): boolean {
  return expandedTools.value.has(toolId)
}

// Format JSON for display
function formatJson(obj: unknown): string {
  try {
    return JSON.stringify(obj, null, 2)
  } catch {
    return String(obj)
  }
}

// Get short preview of arguments for collapsed view
function getArgsPreview(args: Record<string, unknown> | undefined): string {
  if (!args || Object.keys(args).length === 0) return ''
  const entries = Object.entries(args)
  const preview = entries.slice(0, 2).map(([k, v]) => {
    const val = typeof v === 'string' ? v : JSON.stringify(v)
    const shortVal = val.length > 20 ? val.substring(0, 20) + '...' : val
    return `${k}=${shortVal}`
  }).join(', ')
  return entries.length > 2 ? preview + ', ...' : preview
}

const emit = defineEmits<{
  (e: 'regenerate', messageId: string): void
  (e: 'fork', messageId: string): void
  (e: 'copy', content: string): void
}>()

// State for thinking panel
const showThinking = ref(false)

// Configure marked
marked.setOptions({
  breaks: true,
  gfm: true,
})

// Custom renderer for code blocks
const renderer = new marked.Renderer()
renderer.code = ({ text, lang }) => {
  const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
  const highlighted = hljs.highlight(text, { language }).value
  return `<div class="code-block relative group my-3">
    <div class="flex items-center justify-between px-3 py-1.5 bg-gray-200 dark:bg-gray-700 rounded-t-lg text-xs">
      <span class="text-gray-600 dark:text-gray-400">${language}</span>
      <button class="copy-btn opacity-0 group-hover:opacity-100 transition-opacity text-gray-500 hover:text-gray-700 dark:hover:text-gray-300" onclick="navigator.clipboard.writeText(this.closest('.code-block').querySelector('code').textContent)">
        <i class="pi pi-copy"></i>
      </button>
    </div>
    <pre class="!mt-0 !rounded-t-none"><code class="language-${language}">${highlighted}</code></pre>
  </div>`
}

marked.use({ renderer })

// Render LaTeX math expressions
function renderMath(text: string): { text: string; mathPlaceholders: Map<string, string> } {
  const mathPlaceholders: Map<string, string> = new Map()
  let placeholderIndex = 0

  const placeholder = () => `%%MATH_PLACEHOLDER_${placeholderIndex++}%%`

  // Process display math: \[...\] or $$...$$
  text = text.replace(/\\\[([\s\S]*?)\\\]/g, (_, math) => {
    const key = placeholder()
    try {
      mathPlaceholders.set(key, katex.renderToString(math.trim(), { displayMode: true, throwOnError: false }))
    } catch {
      mathPlaceholders.set(key, `<span class="text-red-500">[Math Error]</span>`)
    }
    return key
  })

  text = text.replace(/\$\$([\s\S]*?)\$\$/g, (_, math) => {
    const key = placeholder()
    try {
      mathPlaceholders.set(key, katex.renderToString(math.trim(), { displayMode: true, throwOnError: false }))
    } catch {
      mathPlaceholders.set(key, `<span class="text-red-500">[Math Error]</span>`)
    }
    return key
  })

  // Process inline math: \(...\) or $...$
  text = text.replace(/\\\(([\s\S]*?)\\\)/g, (_, math) => {
    const key = placeholder()
    try {
      mathPlaceholders.set(key, katex.renderToString(math.trim(), { displayMode: false, throwOnError: false }))
    } catch {
      mathPlaceholders.set(key, `<span class="text-red-500">[Math Error]</span>`)
    }
    return key
  })

  // Single $ for inline math (be careful not to match currency)
  text = text.replace(/\$([^$\n]+?)\$/g, (_, math) => {
    if (/^\d+([.,]\d+)?$/.test(math.trim())) {
      return `$${math}$`
    }
    const key = placeholder()
    try {
      mathPlaceholders.set(key, katex.renderToString(math.trim(), { displayMode: false, throwOnError: false }))
    } catch {
      mathPlaceholders.set(key, `<span class="text-red-500">[Math Error]</span>`)
    }
    return key
  })

  return { text, mathPlaceholders }
}

// Process content with math rendering
function processWithMath(content: string): string {
  const { text, mathPlaceholders } = renderMath(content)

  let html = marked.parse(text) as string

  for (const [key, value] of mathPlaceholders) {
    html = html.replace(key, value)
  }

  return DOMPurify.sanitize(html, { ADD_TAGS: ['semantics', 'annotation'], ADD_ATTR: ['encoding'] })
}

// Parse thinking content from <think>...</think> tags
const parsedContent = computed(() => {
  if (!props.message.content) return { thinking: '', answer: '' }

  const content = props.message.content

  // Match <think>...</think> tags (can be multiline)
  const thinkRegex = /<think>([\s\S]*?)<\/think>/gi
  const thinkMatches = content.match(thinkRegex)

  let thinking = ''
  let answer = content

  if (thinkMatches) {
    // Extract all thinking blocks
    thinking = thinkMatches
      .map(match => match.replace(/<\/?think>/gi, '').trim())
      .join('\n\n')

    // Remove thinking blocks from answer
    answer = content.replace(thinkRegex, '').trim()
  }

  return { thinking, answer }
})

const hasThinking = computed(() => parsedContent.value.thinking.length > 0)

// Auto-expand thinking when streaming
watch([() => props.isStreaming, hasThinking], ([streaming, hasTh]) => {
  if (streaming && hasTh) {
    showThinking.value = true
  }
}, { immediate: true })

const renderedThinking = computed(() => {
  if (!parsedContent.value.thinking) return ''
  return processWithMath(parsedContent.value.thinking)
})

const renderedContent = computed(() => {
  if (!parsedContent.value.answer) return ''
  return processWithMath(parsedContent.value.answer)
})

const isUser = computed(() => props.message.role === 'user')

function formatTokens(n: number): string {
  if (n >= 1000) {
    return (n / 1000).toFixed(1) + 'k'
  }
  return n.toString()
}
</script>

<template>
  <div
    :class="[
      'flex gap-4 p-4',
      isUser ? 'bg-white dark:bg-gray-900' : 'bg-gray-50 dark:bg-gray-800/50',
    ]"
  >
    <!-- Avatar -->
    <div
      :class="[
        'w-8 h-8 rounded-full flex items-center justify-center text-white text-sm font-medium flex-shrink-0',
        isUser ? 'bg-blue-500' : 'bg-purple-500',
      ]"
    >
      {{ isUser ? 'U' : 'A' }}
    </div>

    <!-- Content -->
    <div class="flex-1 min-w-0">
      <div class="flex items-center gap-2 mb-1">
        <span class="font-medium">{{ isUser ? 'Vy' : 'Asistent' }}</span>
        <span v-if="message.created_at" class="text-xs text-gray-500">
          {{ new Date(message.created_at).toLocaleTimeString() }}
        </span>
      </div>

      <!-- Attachments -->
      <div v-if="message.attachments?.length" class="flex flex-wrap gap-2 mb-2">
        <div
          v-for="att in message.attachments"
          :key="att.id"
          class="flex items-center gap-2 px-3 py-1.5 bg-gray-100 dark:bg-gray-700 rounded-lg text-sm"
        >
          <i class="pi pi-file"></i>
          <span class="truncate max-w-[9.375rem]">{{ att.filename }}</span>
        </div>
      </div>

      <!-- Thinking content (collapsible) -->
      <div v-if="hasThinking && !isUser" class="mb-3">
        <button
          @click="showThinking = !showThinking"
          class="flex items-center gap-2 text-sm text-purple-600 dark:text-purple-400 hover:text-purple-700 dark:hover:text-purple-300 transition-colors"
        >
          <i :class="['pi', showThinking ? 'pi-chevron-down' : 'pi-chevron-right']"></i>
          <i class="pi pi-lightbulb"></i>
          <span>Přemýšlení</span>
          <span class="text-xs text-gray-500">({{ parsedContent.thinking.length }} znaků)</span>
        </button>
        <div
          v-if="showThinking"
          class="mt-2 p-3 bg-purple-50 dark:bg-purple-900/20 border-l-4 border-purple-400 dark:border-purple-600 rounded-r-lg"
        >
          <div
            class="message-content prose dark:prose-invert prose-sm max-w-none text-gray-600 dark:text-gray-400"
            v-html="renderedThinking"
          />
        </div>
      </div>

      <!-- Streaming thinking indicator -->
      <div
        v-if="isStreaming && !parsedContent.answer && parsedContent.thinking"
        class="mb-3 flex items-center gap-2 text-sm text-purple-600 dark:text-purple-400"
      >
        <i class="pi pi-spin pi-spinner"></i>
        <i class="pi pi-lightbulb"></i>
        <span>Přemýšlím...</span>
      </div>

      <!-- Tool calls (collapsible) -->
      <div v-if="hasToolCalls && !isUser" class="mb-3">
        <button
          @click="showToolCalls = !showToolCalls"
          class="flex items-center gap-2 text-sm text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 transition-colors"
        >
          <i :class="['pi', showToolCalls ? 'pi-chevron-down' : 'pi-chevron-right']"></i>
          <i class="pi pi-wrench"></i>
          <span>MCP nástroje</span>
          <span class="text-xs text-gray-500">({{ toolCalls.length }})</span>
          <!-- Show iteration info if multiple iterations -->
          <span v-if="hasMultipleIterations" class="text-xs bg-purple-100 dark:bg-purple-900/50 text-purple-700 dark:text-purple-300 px-2 py-0.5 rounded-full">
            {{ uniqueIterations.length }} iterací
          </span>
          <!-- Current iteration indicator when streaming -->
          <span v-if="isStreaming && currentIteration && currentIteration > 1" class="text-xs bg-yellow-100 dark:bg-yellow-900/50 text-yellow-700 dark:text-yellow-300 px-2 py-0.5 rounded-full animate-pulse">
            Iterace {{ currentIteration }}/{{ maxIterations }}
          </span>
        </button>
        <div v-if="showToolCalls" class="mt-2 space-y-2">
          <!-- Group by iteration if multiple -->
          <template v-if="hasMultipleIterations">
            <div v-for="iteration in uniqueIterations" :key="iteration" class="space-y-1">
              <!-- Iteration header -->
              <div class="flex items-center gap-2 text-xs text-purple-600 dark:text-purple-400 py-1">
                <i class="pi pi-sync"></i>
                <span class="font-medium">Iterace {{ iteration }}</span>
                <span class="text-gray-400">({{ toolCallsByIteration[iteration]?.length || 0 }} nástrojů)</span>
              </div>
              <!-- Tools in this iteration -->
              <div
                v-for="tc in toolCallsByIteration[iteration]"
                :key="tc.id"
                class="bg-blue-50 dark:bg-blue-900/20 border-l-4 rounded-r-lg overflow-hidden ml-4"
                :class="tc.status === 'running' ? 'border-yellow-400' : tc.status === 'error' ? 'border-red-400' : 'border-green-400'"
              >
                <!-- Tool header (always visible, clickable to expand) -->
                <button
                  @click="toggleToolExpanded(tc.id)"
                  class="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-blue-100 dark:hover:bg-blue-900/40 transition-colors"
                >
                  <i
                    :class="[
                      'pi text-sm',
                      tc.status === 'running' ? 'pi-spin pi-spinner text-yellow-500' :
                      tc.status === 'error' ? 'pi-times-circle text-red-500' :
                      'pi-check-circle text-green-500'
                    ]"
                  ></i>
                  <span class="font-medium text-sm text-gray-900 dark:text-white">{{ tc.name }}</span>
                  <span v-if="getArgsPreview(tc.arguments)" class="text-xs text-gray-500 truncate flex-1">
                    ({{ getArgsPreview(tc.arguments) }})
                  </span>
                  <i :class="['pi text-xs text-gray-400', isToolExpanded(tc.id) ? 'pi-chevron-up' : 'pi-chevron-down']"></i>
                </button>

                <!-- Expanded content -->
                <div v-if="isToolExpanded(tc.id)" class="px-3 pb-3 border-t border-blue-100 dark:border-blue-800">
                  <!-- Arguments -->
                  <div v-if="Object.keys(tc.arguments || {}).length > 0" class="mt-2">
                    <div class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">Argumenty:</div>
                    <pre class="text-xs bg-gray-100 dark:bg-gray-800 p-2 rounded overflow-x-auto max-h-32"><code>{{ formatJson(tc.arguments) }}</code></pre>
                  </div>

                  <!-- Result -->
                  <div v-if="tc.result" class="mt-2">
                    <div class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">Výsledek:</div>
                    <pre class="text-xs bg-gray-100 dark:bg-gray-800 p-2 rounded overflow-x-auto max-h-48"><code>{{ tc.result }}</code></pre>
                  </div>

                  <!-- Error -->
                  <div v-if="tc.error && !tc.result" class="mt-2">
                    <div class="text-xs font-medium text-red-500 mb-1">Chyba:</div>
                    <pre class="text-xs bg-red-100 dark:bg-red-900/30 p-2 rounded overflow-x-auto text-red-700 dark:text-red-300">{{ tc.error }}</pre>
                  </div>
                </div>
              </div>
            </div>
          </template>

          <!-- Single iteration - flat list -->
          <template v-else>
            <div
              v-for="tc in toolCalls"
              :key="tc.id"
              class="bg-blue-50 dark:bg-blue-900/20 border-l-4 rounded-r-lg overflow-hidden"
              :class="tc.status === 'running' ? 'border-yellow-400' : tc.status === 'error' ? 'border-red-400' : 'border-green-400'"
            >
              <!-- Tool header (always visible, clickable to expand) -->
              <button
                @click="toggleToolExpanded(tc.id)"
                class="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-blue-100 dark:hover:bg-blue-900/40 transition-colors"
              >
                <i
                  :class="[
                    'pi text-sm',
                    tc.status === 'running' ? 'pi-spin pi-spinner text-yellow-500' :
                    tc.status === 'error' ? 'pi-times-circle text-red-500' :
                    'pi-check-circle text-green-500'
                  ]"
                ></i>
                <span class="font-medium text-sm text-gray-900 dark:text-white">{{ tc.name }}</span>
                <span v-if="getArgsPreview(tc.arguments)" class="text-xs text-gray-500 truncate flex-1">
                  ({{ getArgsPreview(tc.arguments) }})
                </span>
                <i :class="['pi text-xs text-gray-400', isToolExpanded(tc.id) ? 'pi-chevron-up' : 'pi-chevron-down']"></i>
              </button>

              <!-- Expanded content -->
              <div v-if="isToolExpanded(tc.id)" class="px-3 pb-3 border-t border-blue-100 dark:border-blue-800">
                <!-- Arguments -->
                <div v-if="Object.keys(tc.arguments || {}).length > 0" class="mt-2">
                  <div class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">Argumenty:</div>
                  <pre class="text-xs bg-gray-100 dark:bg-gray-800 p-2 rounded overflow-x-auto max-h-32"><code>{{ formatJson(tc.arguments) }}</code></pre>
                </div>

                <!-- Result -->
                <div v-if="tc.result" class="mt-2">
                  <div class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">Výsledek:</div>
                  <pre class="text-xs bg-gray-100 dark:bg-gray-800 p-2 rounded overflow-x-auto max-h-48"><code>{{ tc.result }}</code></pre>
                </div>

                <!-- Error -->
                <div v-if="tc.error && !tc.result" class="mt-2">
                  <div class="text-xs font-medium text-red-500 mb-1">Chyba:</div>
                  <pre class="text-xs bg-red-100 dark:bg-red-900/30 p-2 rounded overflow-x-auto text-red-700 dark:text-red-300">{{ tc.error }}</pre>
                </div>
              </div>
            </div>
          </template>
        </div>
      </div>

      <!-- Waiting for response indicator -->
      <div
        v-if="isStreaming && !parsedContent.thinking && !parsedContent.answer && !hasToolCalls"
        class="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400"
      >
        <i class="pi pi-spin pi-spinner"></i>
        <span>Čekám na odpověď...</span>
      </div>

      <!-- Message content -->
      <div
        v-else-if="parsedContent.answer || !isStreaming"
        class="message-content prose dark:prose-invert max-w-none"
        :class="{ 'streaming-cursor': isStreaming && parsedContent.answer }"
        v-html="renderedContent"
      />

      <!-- Metrics & Actions (for assistant messages) -->
      <!-- Metriky zobrazují statistiky o zpracování požadavku LLM modelem -->
      <div v-if="!isUser && message.metrics" class="mt-3 flex items-center gap-4 text-xs text-gray-500">
        <span
          v-if="message.metrics.input_tokens != null"
          v-tooltip="'Vstupní tokeny: Počet tokenů odeslaných modelu (systémový prompt + historie + váš dotaz)'"
          class="cursor-help"
        >
          <i class="pi pi-arrow-right mr-1"></i>
          {{ formatTokens(message.metrics.input_tokens) }} in
        </span>
        <span
          v-if="message.metrics.output_tokens != null"
          v-tooltip="'Výstupní tokeny: Počet tokenů vygenerovaných modelem (odpověď)'"
          class="cursor-help"
        >
          <i class="pi pi-arrow-left mr-1"></i>
          {{ formatTokens(message.metrics.output_tokens) }} out
        </span>
        <span
          v-if="message.metrics.ttfb_ms != null"
          v-tooltip="'Doba do prvního tokenu (TTFB): Jak rychle model začal odpovídat'"
          class="cursor-help"
        >
          <i class="pi pi-clock mr-1"></i>
          {{ message.metrics.ttfb_ms.toFixed(0) }}ms TTFB
        </span>
        <span
          v-if="message.metrics.tokens_per_second != null"
          v-tooltip="'Rychlost generování: Kolik tokenů model vygeneruje za sekundu'"
          class="cursor-help"
        >
          <i class="pi pi-bolt mr-1"></i>
          {{ message.metrics.tokens_per_second.toFixed(1) }} tok/s
        </span>
      </div>

      <!-- Actions -->
      <div v-if="!isUser && !isStreaming" class="mt-2 flex items-center gap-1">
        <Button
          icon="pi pi-copy"
          @click="emit('copy', message.content)"
          text
          rounded
          severity="secondary"
          size="small"
          v-tooltip="'Kopírovat'"
        />
        <Button
          icon="pi pi-refresh"
          @click="emit('regenerate', message.id)"
          text
          rounded
          severity="secondary"
          size="small"
          v-tooltip="'Vygenerovat znovu'"
        />
        <Button
          icon="pi pi-code-branch"
          @click="emit('fork', message.id)"
          text
          rounded
          severity="secondary"
          size="small"
          v-tooltip="'Vytvořit větev'"
        />
      </div>
    </div>
  </div>
</template>

<style>
/* Import highlight.js theme */
@import 'highlight.js/styles/github-dark.css';

.dark .hljs {
  background: transparent;
}
</style>
