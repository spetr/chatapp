<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { Conversation } from '@/types'
import {
  getContextStats,
  getContextBreakdown,
  compactContext,
  getContextPreview,
  type ContextStats,
  type ContextBreakdown,
  type CompactResult,
  type ContextPreview
} from '@/api/client'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import ProgressBar from 'primevue/progressbar'
import SelectButton from 'primevue/selectbutton'
import Slider from 'primevue/slider'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'

const props = defineProps<{
  visible: boolean
  conversation: Conversation | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'contextCompacted'): void
}>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (value) => emit('update:visible', value)
})

// State
const isLoading = ref(false)
const activeTab = ref('breakdown')
const stats = ref<ContextStats | null>(null)
const breakdown = ref<ContextBreakdown | null>(null)
const preview = ref<ContextPreview | null>(null)
const compactResult = ref<CompactResult | null>(null)

// Compact settings
const compactStrategy = ref<'summarize' | 'drop_oldest' | 'smart'>('summarize')
const keepRecent = ref(10)

const strategyOptions = [
  { label: 'Shrnutí', value: 'summarize' },
  { label: 'Zahodit staré', value: 'drop_oldest' },
  { label: 'Chytré', value: 'smart' }
]

// Load data when dialog opens
watch(() => props.visible, async (visible) => {
  if (visible && props.conversation) {
    await loadAllData()
  }
})

async function loadAllData() {
  if (!props.conversation) return
  isLoading.value = true
  try {
    const [statsData, breakdownData, previewData] = await Promise.all([
      getContextStats(props.conversation.id),
      getContextBreakdown(props.conversation.id),
      getContextPreview(props.conversation.id)
    ])
    stats.value = statsData
    breakdown.value = breakdownData
    preview.value = previewData
    compactResult.value = null
  } catch (error) {
    console.error('Failed to load context data:', error)
  } finally {
    isLoading.value = false
  }
}

async function runCompactPreview() {
  if (!props.conversation) return
  isLoading.value = true
  try {
    compactResult.value = await compactContext(props.conversation.id, {
      strategy: compactStrategy.value,
      keep_recent: keepRecent.value,
      preview_only: true
    })
  } catch (error) {
    console.error('Failed to preview compact:', error)
  } finally {
    isLoading.value = false
  }
}

async function applyCompact() {
  if (!props.conversation) return
  isLoading.value = true
  try {
    compactResult.value = await compactContext(props.conversation.id, {
      strategy: compactStrategy.value,
      keep_recent: keepRecent.value,
      preview_only: false
    })
    // Reload data after compaction
    await loadAllData()
    emit('contextCompacted')
  } catch (error) {
    console.error('Failed to apply compact:', error)
  } finally {
    isLoading.value = false
  }
}

function formatTokens(n: number): string {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
  return n.toString()
}

function getProgressColor(percent: number): string {
  if (percent > 90) return '#ef4444'
  if (percent > 70) return '#f59e0b'
  if (percent > 50) return '#3b82f6'
  return '#22c55e'
}

function getRoleColor(role: string): string {
  switch (role) {
    case 'user': return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
    case 'assistant': return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
    case 'system': return 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
    default: return 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400'
  }
}

function getRoleLabel(role: string): string {
  switch (role) {
    case 'user': return 'U'
    case 'assistant': return 'A'
    case 'system': return 'S'
    default: return '?'
  }
}
</script>

<template>
  <Dialog
    v-model:visible="dialogVisible"
    modal
    :style="{ width: '65rem', maxWidth: '95vw' }"
    :closable="true"
  >
    <template #header>
      <div class="flex items-center justify-between w-full pr-8">
        <div class="modal-header">
          <div class="modal-header-icon">
            <i class="pi pi-database text-white"></i>
          </div>
          <div class="modal-header-text">
            <h2>Správa kontextu</h2>
            <p>Optimalizujte využití tokenů a náklady</p>
          </div>
        </div>
        <!-- Compact stats in header -->
        <div v-if="stats" class="flex items-center gap-4">
          <div class="flex items-center gap-6 text-sm">
            <div class="text-center">
              <div class="font-bold font-mono text-lg">{{ formatTokens(stats.estimated_tokens) }}</div>
              <div class="text-xs text-gray-500">tokenů</div>
            </div>
            <div class="text-center">
              <div class="font-bold font-mono text-lg">{{ stats.message_count }}</div>
              <div class="text-xs text-gray-500">zpráv</div>
            </div>
            <div class="text-center">
              <div class="font-bold font-mono text-lg" :class="stats.token_percent_used > 80 ? 'text-red-500' : stats.token_percent_used > 60 ? 'text-yellow-500' : 'text-green-500'">
                {{ stats.token_percent_used.toFixed(0) }}%
              </div>
              <div class="text-xs text-gray-500">využito</div>
            </div>
          </div>
          <div class="w-32">
            <ProgressBar
              :value="Math.min(stats.token_percent_used, 100)"
              :showValue="false"
              :style="{ height: '0.5rem' }"
              :pt="{ value: { style: { backgroundColor: getProgressColor(stats.token_percent_used) } } }"
            />
          </div>
        </div>
      </div>
    </template>
    <div v-if="isLoading" class="flex items-center justify-center py-8">
      <i class="pi pi-spin pi-spinner text-2xl" />
    </div>

    <div v-else-if="stats">
      <!-- Tabs -->
      <div class="context-tabs-wrapper">
        <Tabs v-model:value="activeTab">
          <TabList>
            <Tab value="breakdown">Rozpad tokenů</Tab>
            <Tab value="compact">Kompaktovat</Tab>
            <Tab value="preview">Náhled API</Tab>
          </TabList>
          <TabPanels>
            <!-- Token Breakdown Tab -->
            <TabPanel value="breakdown">
            <div class="space-y-3">
              <!-- Recommendations -->
              <div v-if="stats.recommendations.length > 0" class="space-y-1">
                <div
                  v-for="(rec, index) in stats.recommendations"
                  :key="index"
                  class="flex items-start gap-2 text-sm p-2 rounded"
                  :class="[
                    stats.status === 'critical' ? 'bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300' :
                    stats.status === 'warning' ? 'bg-yellow-50 dark:bg-yellow-900/20 text-yellow-700 dark:text-yellow-300' :
                    'bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-300'
                  ]"
                >
                  <i class="pi pi-lightbulb mt-0.5" />
                  <span>{{ rec }}</span>
                </div>
              </div>

              <!-- Message breakdown -->
              <div v-if="breakdown" class="space-y-1">
              <div
                v-for="msg in breakdown.breakdown"
                :key="msg.id"
                class="flex items-center gap-2 p-2 rounded hover:bg-gray-50 dark:hover:bg-gray-800"
              >
                <!-- Role badge -->
                <span
                  class="w-6 h-6 flex items-center justify-center rounded text-xs font-bold"
                  :class="getRoleColor(msg.role)"
                >
                  {{ getRoleLabel(msg.role) }}
                </span>

                <!-- Token bar -->
                <div class="flex-1">
                  <div class="flex items-center gap-2">
                    <div
                      class="h-4 rounded"
                      :style="{
                        width: `${Math.max(msg.percent, 2)}%`,
                        backgroundColor: getProgressColor(msg.percent * 2)
                      }"
                    />
                    <span class="text-xs font-mono whitespace-nowrap">
                      {{ msg.tokens }} ({{ msg.percent.toFixed(1) }}%)
                    </span>
                  </div>
                  <div class="text-xs text-gray-500 truncate mt-0.5">
                    {{ msg.content_preview }}
                  </div>
                </div>

                <!-- Attachment indicator -->
                <span v-if="msg.has_attachments" class="text-xs text-gray-400">
                  <i class="pi pi-paperclip" /> {{ msg.attachment_count }}
                </span>
              </div>
              </div>
            </div>
          </TabPanel>

          <!-- Compact Tab -->
          <TabPanel value="compact">
          <div class="space-y-4">
            <!-- Explanation Box -->
            <div class="info-box info-box-blue">
              <div class="flex items-start gap-2">
                <i class="pi pi-info-circle info-box-icon mt-0.5"></i>
                <div>
                  <p class="info-box-title mb-1">Co je kompaktování kontextu?</p>
                  <p class="info-box-text text-xs">
                    Každá zpráva v konverzaci zabírá tokeny. Při dlouhých konverzacích náklady rostou,
                    protože s každým dotazem se odesílá celá historie. Kompaktováním můžete zredukovat
                    historii a ušetřit tokeny (a peníze) bez ztráty důležitého kontextu.
                  </p>
                </div>
              </div>
            </div>

            <!-- Strategy selection -->
            <div class="form-field">
              <label class="form-label">Strategie kompaktování</label>
              <SelectButton
                v-model="compactStrategy"
                :options="strategyOptions"
                optionLabel="label"
                optionValue="value"
              />
            </div>

            <!-- Strategy explanation boxes -->
            <div class="grid gap-3">
              <div
                class="selection-card"
                :class="{ 'selected': compactStrategy === 'summarize' }"
                @click="compactStrategy = 'summarize'"
              >
                <div class="selection-card-header">
                  <i class="pi pi-file-edit icon-purple"></i>
                  <span class="selection-card-title">Shrnutí</span>
                </div>
                <p class="selection-card-desc">
                  <strong>Jak funguje:</strong> Staré zprávy se analyzují a vytvoří se shrnutí klíčových bodů konverzace.
                </p>
                <div class="flex gap-4 mt-2 text-xs">
                  <span class="text-green-600 dark:text-green-400"><i class="pi pi-check mr-1"></i>Zachová kontext</span>
                  <span class="text-yellow-600 dark:text-yellow-400"><i class="pi pi-exclamation-triangle mr-1"></i>Ztrácí detaily</span>
                </div>
              </div>

              <div
                class="selection-card"
                :class="{ 'selected': compactStrategy === 'drop_oldest' }"
                @click="compactStrategy = 'drop_oldest'"
              >
                <div class="selection-card-header">
                  <i class="pi pi-trash icon-danger"></i>
                  <span class="selection-card-title">Zahodit staré</span>
                </div>
                <p class="selection-card-desc">
                  <strong>Jak funguje:</strong> Nejstarší zprávy se jednoduše smažou. Žádné shrnutí se nevytváří.
                </p>
                <div class="flex gap-4 mt-2 text-xs">
                  <span class="text-green-600 dark:text-green-400"><i class="pi pi-check mr-1"></i>Nejrychlejší</span>
                  <span class="text-red-600 dark:text-red-400"><i class="pi pi-times mr-1"></i>Ztráta kontextu</span>
                </div>
              </div>

              <div
                class="selection-card"
                :class="{ 'selected': compactStrategy === 'smart' }"
                @click="compactStrategy = 'smart'"
              >
                <div class="selection-card-header">
                  <i class="pi pi-sparkles icon-success"></i>
                  <span class="selection-card-title">Chytré</span>
                </div>
                <p class="selection-card-desc">
                  <strong>Jak funguje:</strong> Analyzuje zprávy a zachová ty důležité (otázky, klíčová rozhodnutí, přílohy).
                  Ostatní shrne.
                </p>
                <div class="flex gap-4 mt-2 text-xs">
                  <span class="text-green-600 dark:text-green-400"><i class="pi pi-check mr-1"></i>Nejlepší poměr</span>
                  <span class="text-blue-600 dark:text-blue-400"><i class="pi pi-clock mr-1"></i>Trvá déle</span>
                </div>
              </div>
            </div>

            <!-- Keep recent slider -->
            <div>
              <label class="block text-sm font-medium mb-2">
                Ponechat posledních {{ keepRecent }} zpráv
              </label>
              <Slider v-model="keepRecent" :min="2" :max="50" class="w-full" />
            </div>

            <!-- Preview button -->
            <Button
              label="Náhled kompaktování"
              icon="pi pi-eye"
              @click="runCompactPreview"
              :loading="isLoading"
              severity="secondary"
              class="w-full"
            />

            <!-- Compact result preview -->
            <div v-if="compactResult" class="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg space-y-3">
              <div class="flex items-center gap-2">
                <i
                  :class="[
                    'pi',
                    compactResult.status === 'applied' ? 'pi-check-circle text-green-500' :
                    compactResult.status === 'no_change' ? 'pi-info-circle text-blue-500' :
                    'pi-eye text-purple-500'
                  ]"
                />
                <span class="font-medium">
                  {{ compactResult.status === 'applied' ? 'Kompaktováno!' :
                     compactResult.status === 'no_change' ? 'Žádná změna' :
                     'Náhled kompaktování' }}
                </span>
              </div>

              <div class="grid grid-cols-2 gap-4 text-sm">
                <div>
                  <span class="text-gray-500">Původně:</span>
                  <span class="font-mono ml-2">{{ formatTokens(compactResult.original_tokens) }}</span>
                </div>
                <div>
                  <span class="text-gray-500">Po kompaktování:</span>
                  <span class="font-mono ml-2">{{ formatTokens(compactResult.new_tokens) }}</span>
                </div>
                <div>
                  <span class="text-gray-500">Ušetřeno:</span>
                  <span class="font-mono ml-2 text-green-600">
                    {{ formatTokens(compactResult.tokens_saved) }}
                    ({{ compactResult.percent_saved.toFixed(0) }}%)
                  </span>
                </div>
                <div>
                  <span class="text-gray-500">Zpráv odstraněno:</span>
                  <span class="font-mono ml-2">{{ compactResult.messages_removed }}</span>
                </div>
              </div>

              <!-- Summary preview -->
              <div v-if="compactResult.summary" class="mt-3">
                <div class="text-xs font-medium text-gray-500 mb-1">Shrnutí:</div>
                <div class="text-sm p-2 bg-white dark:bg-gray-900 rounded border border-gray-200 dark:border-gray-700">
                  {{ compactResult.summary }}
                </div>
              </div>

              <!-- Apply button -->
              <Button
                v-if="compactResult.status === 'preview' && compactResult.tokens_saved > 0"
                label="Aplikovat kompaktování"
                icon="pi pi-check"
                @click="applyCompact"
                :loading="isLoading"
                severity="primary"
                class="w-full mt-2"
              />
            </div>
          </div>
          </TabPanel>

          <!-- Context Preview Tab -->
          <TabPanel value="preview">
            <div v-if="preview" class="space-y-3">
              <div class="flex items-center justify-between text-sm">
                <span>
                  Zpráv k odeslání: <strong>{{ preview.message_count }}</strong>
                  <span v-if="preview.was_truncated" class="text-yellow-600 ml-2">
                    (zkráceno z {{ preview.original_count }})
                  </span>
                </span>
                <span class="font-mono">{{ formatTokens(preview.total_tokens) }} tokenů</span>
              </div>

              <div class="space-y-2">
                <div
                  v-for="(msg, index) in preview.messages"
                  :key="index"
                  class="p-3 rounded-lg border"
                  :class="[
                    msg.role === 'system' ? 'bg-purple-50 dark:bg-purple-900/20 border-purple-200 dark:border-purple-800' :
                    msg.role === 'user' ? 'bg-blue-50 dark:bg-blue-900/20 border-blue-200 dark:border-blue-800' :
                    'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800'
                  ]"
                >
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-xs font-medium uppercase" :class="getRoleColor(msg.role).split(' ')[1]">
                      {{ msg.role }}
                    </span>
                    <span class="text-xs font-mono text-gray-500">{{ msg.tokens }} tok</span>
                  </div>
                  <div class="text-sm whitespace-pre-wrap break-words">
                    {{ msg.content.length > 500 ? msg.content.slice(0, 500) + '...' : msg.content }}
                  </div>
                </div>
              </div>

              <div v-if="preview.max_history_length > 0" class="text-xs text-gray-500">
                <i class="pi pi-info-circle mr-1"></i>
                Max historie: {{ preview.max_history_length }} zpráv (sliding window)
              </div>
            </div>
          </TabPanel>
          </TabPanels>
        </Tabs>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-between items-center">
        <Button
          label="Obnovit"
          icon="pi pi-refresh"
          @click="loadAllData"
          :loading="isLoading"
          text
        />
        <Button
          label="Zavřít"
          @click="dialogVisible = false"
          severity="secondary"
        />
      </div>
    </template>
  </Dialog>
</template>

<style scoped>
/* Wrapper with fixed height */
.context-tabs-wrapper {
  height: 34rem; /* 544px */
  display: flex;
  flex-direction: column;
}

.context-tabs-wrapper :deep(.p-tabs) {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.context-tabs-wrapper :deep(.p-tablist) {
  flex-shrink: 0;
}

.context-tabs-wrapper :deep(.p-tabpanels) {
  flex: 1;
  overflow: hidden;
}

.context-tabs-wrapper :deep(.p-tabpanel) {
  height: 100%;
  overflow-y: auto;
  overflow-x: hidden;
  min-height: unset !important;
}

/* Custom scrollbar */
.context-tabs-wrapper :deep(.p-tabpanel)::-webkit-scrollbar {
  width: 0.375rem; /* 6px */
}

.context-tabs-wrapper :deep(.p-tabpanel)::-webkit-scrollbar-track {
  @apply bg-gray-100 dark:bg-gray-800 rounded-full;
}

.context-tabs-wrapper :deep(.p-tabpanel)::-webkit-scrollbar-thumb {
  @apply bg-gray-300 dark:bg-gray-600 rounded-full;
}

.context-tabs-wrapper :deep(.p-tabpanel)::-webkit-scrollbar-thumb:hover {
  @apply bg-gray-400 dark:bg-gray-500;
}
</style>
