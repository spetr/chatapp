<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useChatStore } from '@/stores/chat'
import type { Conversation, ConversationSettings, ModelInfo } from '@/types'
import Dialog from 'primevue/dialog'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Slider from 'primevue/slider'
import ToggleSwitch from 'primevue/toggleswitch'
import Button from 'primevue/button'
import Divider from 'primevue/divider'

const props = defineProps<{
  visible: boolean
  conversation?: Conversation | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'create', provider: string, model: string, systemPrompt: string, settings: ConversationSettings): void
  (e: 'save', model: string, systemPrompt: string, settings: ConversationSettings): void
}>()

const chatStore = useChatStore()

const isEditMode = computed(() => !!props.conversation)

// Tab state
const activeTab = ref('basic')

// Basic settings
const selectedProvider = ref('')
const selectedModel = ref('')
const systemPrompt = ref('')
const selectedPromptTemplate = ref('')

// Generation settings
const temperature = ref<number | null>(null)
const maxTokens = ref<number | null>(null)
const topP = ref<number | null>(null)
const topK = ref<number | null>(null)
const frequencyPenalty = ref<number | null>(null)
const presencePenalty = ref<number | null>(null)
const repeatPenalty = ref<number | null>(null)

// Feature toggles
const enableStreaming = ref(true)
const enableThinking = ref(false)
const enableTools = ref(false)

// Context settings
const contextLength = ref<number | null>(null)
const maxHistoryLength = ref<number | null>(null)

// Advanced settings
const responseFormat = ref('text')
const thinkingBudget = ref<string>('medium') // low, medium, high
const numCtx = ref<number | null>(null)
const numPredict = ref<number | null>(null)
const stopSequences = ref('')
const seed = ref<number | null>(null)
const grammar = ref('')

// ReAct settings
const maxToolIterations = ref<number | null>(null)

// Dynamic models for local providers (Ollama/llama.cpp fetch their own models)
interface DynamicModelDetail {
  name: string
  size?: number
  family?: string
  parameter_size?: string
  supports_thinking: boolean
}

const dynamicModels = ref<string[]>([])
const dynamicModelDetails = ref<Map<string, DynamicModelDetail>>(new Map())
const isLoadingModels = ref(false)

// Provider info (descriptions only - pricing comes from model registry)
const providerInfo: Record<string, { description: string }> = {
  claude: {
    description: 'Modely Claude od Anthropic. Vynikají v komplexním uvažování a programování.',
  },
  openai: {
    description: 'Modely GPT od OpenAI. Průmyslový standard se silnými schopnostmi.',
  },
  ollama: {
    description: 'Lokální modely přes Ollama. Běží zdarma na vašem počítači.',
  },
  llamacpp: {
    description: 'Přímé připojení k llama.cpp serveru. Plná kontrola nad inference parametry.',
  },
}

// Get pricing text for the current model
const currentModelPricing = computed(() => {
  const model = currentModelInfo.value as ModelInfo | undefined
  if (!model?.pricing) {
    // Local providers don't have pricing
    if (selectedProvider.value === 'ollama' || selectedProvider.value === 'llamacpp') {
      return 'Zdarma (pouze náklady na elektřinu)'
    }
    return null
  }
  return `$${model.pricing.input_per_1m}/$${model.pricing.output_per_1m} za 1M tokenů (vstup/výstup)`
})

const providerOptions = computed(() => {
  return chatStore.providers.map((p) => ({
    label: p.name,
    value: p.id,
  }))
})

const modelOptions = computed(() => {
  const provider = chatStore.providers.find((p) => p.id === selectedProvider.value)
  if (!provider) return []

  // For local providers with dynamic model fetching
  if (dynamicModels.value.length > 0) {
    return dynamicModels.value.map((m) => {
      const detail = dynamicModelDetails.value.get(m)
      return {
        label: m,
        value: m,
        displayName: m,
        detail,
        supportsThinking: detail?.supports_thinking ?? false,
      }
    })
  }

  // Use models from the store registry
  const registryModels = chatStore.getModelsForProvider(selectedProvider.value)
  return registryModels.map((m: ModelInfo) => ({
    label: m.display_name,
    value: m.id,
    displayName: m.display_name,
    model: m,
    supportsThinking: m.capabilities?.thinking ?? false,
  }))
})

const currentProviderInfo = computed(() => providerInfo[selectedProvider.value])

// Get current model info - either from dynamic fetch or registry
const currentModelInfo = computed(() => {
  // First check dynamic models (for local providers)
  if (dynamicModelDetails.value.has(selectedModel.value)) {
    return dynamicModelDetails.value.get(selectedModel.value)
  }
  // Then check registry
  return chatStore.models.find(m => m.id === selectedModel.value)
})

const promptOptions = computed(() => {
  return [
    { label: 'Vlastní', value: '' },
    ...chatStore.prompts.map((p) => ({
      label: p.name,
      value: p.id,
    })),
  ]
})

const supportsModelDetection = computed(() => {
  return selectedProvider.value === 'ollama' || selectedProvider.value === 'openai' || selectedProvider.value === 'llamacpp'
})

// Slider value wrappers (Slider doesn't accept null)
const temperatureSlider = computed({
  get: () => temperature.value ?? 1,
  set: (val: number) => { temperature.value = val }
})
const topPSlider = computed({
  get: () => topP.value ?? 1,
  set: (val: number) => { topP.value = val }
})
const frequencyPenaltySlider = computed({
  get: () => frequencyPenalty.value ?? 0,
  set: (val: number) => { frequencyPenalty.value = val }
})
const presencePenaltySlider = computed({
  get: () => presencePenalty.value ?? 0,
  set: (val: number) => { presencePenalty.value = val }
})
const repeatPenaltySlider = computed({
  get: () => repeatPenalty.value ?? 1.1,
  set: (val: number) => { repeatPenalty.value = val }
})
const maxHistorySlider = computed({
  get: () => maxHistoryLength.value ?? 50,
  set: (val: number) => { maxHistoryLength.value = val }
})

// Provider-specific visibility
const isLlamaCpp = computed(() => selectedProvider.value === 'llamacpp')
const isLocalProvider = computed(() => selectedProvider.value === 'ollama' || selectedProvider.value === 'llamacpp')
const isOpenAI = computed(() => selectedProvider.value === 'openai')

async function fetchModels() {
  const provider = chatStore.providers.find((p) => p.id === selectedProvider.value)
  if (!provider) return

  // Only fetch dynamically for local providers
  if (provider.type !== 'local' && provider.id !== 'openai') {
    return
  }

  isLoadingModels.value = true
  dynamicModels.value = []
  dynamicModelDetails.value = new Map()

  try {
    let endpoint = ''
    if (provider.id === 'ollama') {
      endpoint = '/api/ollama/models'
    } else if (provider.id === 'openai') {
      endpoint = '/api/openai/models'
    } else if (provider.id === 'llamacpp') {
      endpoint = '/api/llamacpp/models'
    } else {
      return
    }

    const response = await fetch(endpoint)
    if (response.ok) {
      const data = await response.json()
      if (data.models && data.models.length > 0) {
        dynamicModels.value = data.models
        if (data.model_details) {
          for (const detail of data.model_details) {
            dynamicModelDetails.value.set(detail.name, detail)
          }
        }
        if (!dynamicModels.value.includes(selectedModel.value)) {
          selectedModel.value = dynamicModels.value[0]
        }
      }
    }
  } catch (error) {
    console.error('Failed to fetch models:', error)
  } finally {
    isLoadingModels.value = false
  }
}

// Build settings object from form values
function buildSettings(): ConversationSettings {
  const settings: ConversationSettings = {}

  if (temperature.value !== null) settings.temperature = temperature.value
  if (maxTokens.value !== null) settings.max_tokens = maxTokens.value
  if (topP.value !== null) settings.top_p = topP.value
  if (topK.value !== null) settings.top_k = topK.value
  if (frequencyPenalty.value !== null) settings.frequency_penalty = frequencyPenalty.value
  if (presencePenalty.value !== null) settings.presence_penalty = presencePenalty.value
  if (repeatPenalty.value !== null) settings.repeat_penalty = repeatPenalty.value

  settings.stream = enableStreaming.value
  if (enableThinking.value) {
    settings.enable_thinking = true
    // Include thinking budget when thinking is enabled
    if (thinkingBudget.value) settings.thinking_budget = thinkingBudget.value
  }
  if (enableTools.value) settings.enable_tools = true

  if (contextLength.value !== null) settings.context_length = contextLength.value
  if (maxHistoryLength.value !== null) settings.max_history_length = maxHistoryLength.value

  if (responseFormat.value !== 'text') settings.response_format = responseFormat.value
  if (numCtx.value !== null) settings.num_ctx = numCtx.value
  if (numPredict.value !== null) settings.num_predict = numPredict.value

  if (stopSequences.value.trim()) {
    settings.stop_sequences = stopSequences.value.split(',').map((s) => s.trim()).filter(Boolean)
  }

  if (seed.value !== null) settings.seed = seed.value
  if (grammar.value.trim()) settings.grammar = grammar.value.trim()

  // ReAct settings
  if (maxToolIterations.value !== null) settings.max_tool_iterations = maxToolIterations.value

  return settings
}

// Load settings from existing conversation
function loadSettings(settings?: ConversationSettings) {
  if (!settings) {
    // Reset to defaults
    temperature.value = null
    maxTokens.value = null
    topP.value = null
    topK.value = null
    frequencyPenalty.value = null
    presencePenalty.value = null
    repeatPenalty.value = null
    enableStreaming.value = true
    enableThinking.value = false
    enableTools.value = false
    contextLength.value = null
    maxHistoryLength.value = null
    responseFormat.value = 'text'
    thinkingBudget.value = 'medium'
    numCtx.value = null
    numPredict.value = null
    stopSequences.value = ''
    seed.value = null
    grammar.value = ''
    maxToolIterations.value = null
    return
  }

  temperature.value = settings.temperature ?? null
  maxTokens.value = settings.max_tokens ?? null
  topP.value = settings.top_p ?? null
  topK.value = settings.top_k ?? null
  frequencyPenalty.value = settings.frequency_penalty ?? null
  presencePenalty.value = settings.presence_penalty ?? null
  repeatPenalty.value = settings.repeat_penalty ?? null
  enableStreaming.value = settings.stream ?? true
  enableThinking.value = settings.enable_thinking ?? false
  enableTools.value = settings.enable_tools ?? false
  contextLength.value = settings.context_length ?? null
  maxHistoryLength.value = settings.max_history_length ?? null
  responseFormat.value = settings.response_format ?? 'text'
  thinkingBudget.value = settings.thinking_budget ?? 'medium'
  numCtx.value = settings.num_ctx ?? null
  numPredict.value = settings.num_predict ?? null
  stopSequences.value = settings.stop_sequences?.join(', ') ?? ''
  seed.value = settings.seed ?? null
  grammar.value = settings.grammar ?? ''
  maxToolIterations.value = settings.max_tool_iterations ?? null
}

// Watch for provider change
watch(selectedProvider, (newProvider) => {
  // Clear dynamic models when switching providers
  dynamicModels.value = []
  dynamicModelDetails.value = new Map()

  if (!isEditMode.value) {
    // Get default model from registry
    const defaultModel = chatStore.getDefaultModel(newProvider)
    if (defaultModel) {
      selectedModel.value = defaultModel.id
    } else {
      // Fallback to first model for this provider
      const models = chatStore.getModelsForProvider(newProvider)
      if (models.length > 0) {
        selectedModel.value = models[0].id
      }
    }
  }

  // Fetch dynamic models for local providers and OpenAI
  if (newProvider === 'ollama' || newProvider === 'openai' || newProvider === 'llamacpp') {
    fetchModels()
  }
})

// Watch for prompt template change
watch(selectedPromptTemplate, (templateId) => {
  if (templateId) {
    const template = chatStore.prompts.find((p) => p.id === templateId)
    if (template) {
      systemPrompt.value = template.content
    }
  }
})

// Initialize when dialog opens
watch(
  () => props.visible,
  async (isVisible) => {
    if (isVisible && chatStore.providers.length > 0) {
      dynamicModels.value = []
      dynamicModelDetails.value = new Map()

      if (props.conversation) {
        // Edit mode
        selectedProvider.value = props.conversation.provider
        selectedModel.value = props.conversation.model
        systemPrompt.value = props.conversation.system_prompt || ''
        selectedPromptTemplate.value = ''
        loadSettings(props.conversation.settings)

        if (selectedProvider.value === 'ollama' || selectedProvider.value === 'openai' || selectedProvider.value === 'llamacpp') {
          await fetchModels()
        }
      } else {
        // Create mode - use first available provider
        const availableProviders = chatStore.providers.filter(p => p.available)
        selectedProvider.value = availableProviders.length > 0 ? availableProviders[0].id : chatStore.providers[0].id

        // Get default model from registry
        const defaultModel = chatStore.getDefaultModel(selectedProvider.value)
        if (defaultModel) {
          selectedModel.value = defaultModel.id
        } else {
          const models = chatStore.getModelsForProvider(selectedProvider.value)
          if (models.length > 0) {
            selectedModel.value = models[0].id
          }
        }

        const defaultPrompt = chatStore.prompts.find((p) => p.id === 'default')
        if (defaultPrompt) {
          systemPrompt.value = defaultPrompt.content
          selectedPromptTemplate.value = 'default'
        } else {
          systemPrompt.value = ''
          selectedPromptTemplate.value = ''
        }
        loadSettings()

        // Fetch dynamic models for local providers
        if (selectedProvider.value === 'ollama' || selectedProvider.value === 'openai' || selectedProvider.value === 'llamacpp') {
          await fetchModels()
        }
      }
    }
  }
)

function handleSubmit() {
  const settings = buildSettings()
  if (isEditMode.value) {
    emit('save', selectedModel.value, systemPrompt.value, settings)
  } else {
    emit('create', selectedProvider.value, selectedModel.value, systemPrompt.value, settings)
  }
  emit('update:visible', false)
}
</script>

<template>
  <Dialog
    :visible="visible"
    @update:visible="emit('update:visible', $event)"
    modal
    :style="{ width: '65rem', maxWidth: '95vw' }"
    :breakpoints="{ '768px': '95vw' }"
  >
    <template #header>
      <div class="modal-header">
        <div class="modal-header-icon">
          <i :class="['pi text-white', isEditMode ? 'pi-cog' : 'pi-plus']"></i>
        </div>
        <div class="modal-header-text">
          <h2>{{ isEditMode ? 'Nastavení konverzace' : 'Nová konverzace' }}</h2>
          <p>{{ isEditMode ? 'Upravte parametry a chování' : 'Vyberte poskytovatele a model' }}</p>
        </div>
      </div>
    </template>
    <div class="settings-tabs-wrapper">
      <Tabs v-model:value="activeTab">
        <TabList>
          <Tab value="basic">Základní</Tab>
          <Tab value="generation">Generování</Tab>
          <Tab value="features">Funkce</Tab>
          <Tab value="context">Kontext</Tab>
        </TabList>
        <TabPanels>
        <!-- Basic Tab -->
        <TabPanel value="basic">
          <div class="space-y-4">
          <!-- Provider -->
          <div>
            <label class="block text-sm font-medium mb-2">Poskytovatel</label>
            <Select
              v-model="selectedProvider"
              :options="providerOptions"
              optionLabel="label"
              optionValue="value"
              placeholder="Vyberte poskytovatele"
              class="w-full"
              :disabled="isEditMode"
            />
            <p v-if="isEditMode" class="mt-1 text-xs text-gray-500">
              Poskytovatele nelze změnit u existující konverzace
            </p>
            <div v-if="currentProviderInfo" class="mt-2 p-2 bg-gray-50 dark:bg-gray-800 rounded text-xs">
              <p class="text-gray-600 dark:text-gray-400">{{ currentProviderInfo.description }}</p>
              <p v-if="currentModelPricing" class="mt-1 text-gray-500">{{ currentModelPricing }}</p>
            </div>
          </div>

          <!-- Model -->
          <div>
            <div class="flex items-center justify-between mb-2">
              <label class="text-sm font-medium">Model</label>
              <Button
                v-if="supportsModelDetection"
                icon="pi pi-refresh"
                label="Načíst"
                size="small"
                severity="secondary"
                text
                @click="fetchModels"
                :loading="isLoadingModels"
              />
            </div>
            <Select
              v-model="selectedModel"
              :options="modelOptions"
              optionLabel="label"
              optionValue="value"
              placeholder="Vyberte model"
              class="w-full"
              :disabled="!selectedProvider"
              :loading="isLoadingModels"
            >
              <template #option="{ option }">
                <div class="flex items-center gap-2">
                  <span>{{ option.label }}</span>
                  <i v-if="option.supportsThinking" class="pi pi-lightbulb text-purple-500 text-xs" title="Podporuje přemýšlení"></i>
                </div>
                <div v-if="option.detail" class="text-xs text-gray-500">
                  {{ option.detail.family }} {{ option.detail.parameter_size }}
                </div>
              </template>
            </Select>
            <div v-if="currentModelInfo" class="mt-2 flex flex-wrap gap-2 text-xs">
              <span v-if="currentModelInfo.family" class="px-2 py-1 bg-gray-100 dark:bg-gray-700 rounded">
                {{ currentModelInfo.family }}
              </span>
              <span v-if="(currentModelInfo as DynamicModelDetail).parameter_size" class="px-2 py-1 bg-gray-100 dark:bg-gray-700 rounded">
                {{ (currentModelInfo as DynamicModelDetail).parameter_size }}
              </span>
              <span v-if="(currentModelInfo as ModelInfo).context_window" class="px-2 py-1 bg-gray-100 dark:bg-gray-700 rounded">
                {{ Math.round((currentModelInfo as ModelInfo).context_window / 1000) }}k ctx
              </span>
              <span v-if="(currentModelInfo as DynamicModelDetail).supports_thinking || (currentModelInfo as ModelInfo).capabilities?.thinking" class="px-2 py-1 bg-purple-100 dark:bg-purple-900 text-purple-700 dark:text-purple-300 rounded">
                <i class="pi pi-lightbulb mr-1"></i>Thinking
              </span>
              <span v-if="(currentModelInfo as ModelInfo).capabilities?.vision" class="px-2 py-1 bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 rounded">
                <i class="pi pi-image mr-1"></i>Vision
              </span>
            </div>
          </div>

          <!-- System Prompt -->
          <div>
            <div class="flex items-center justify-between mb-2">
              <label class="text-sm font-medium">Systémový prompt</label>
              <Select
                v-model="selectedPromptTemplate"
                :options="promptOptions"
                optionLabel="label"
                optionValue="value"
                placeholder="Šablona"
                class="w-32"
                size="small"
              />
            </div>
            <Textarea
              v-model="systemPrompt"
              rows="4"
              class="w-full"
              placeholder="Jsi užitečný asistent..."
            />
          </div>
        </div>
        </TabPanel>

        <!-- Generation Tab -->
        <TabPanel value="generation">
          <div class="space-y-5">
          <!-- Educational info box -->
          <div class="info-box info-box-blue">
            <div class="flex items-start gap-2">
              <i class="pi pi-question-circle info-box-icon mt-0.5" />
              <div>
                <p class="info-box-title">Jak funguje generování textu?</p>
                <p class="info-box-text mt-1">
                  LLM modely generují text token po tokenu. Každý token je vybrán z pravděpodobnostního rozdělení.
                  Parametry níže ovlivňují, jak model vybírá tokeny - od deterministického (vždy stejná odpověď)
                  po kreativní (různé, někdy překvapivé odpovědi).
                </p>
              </div>
            </div>
          </div>

          <!-- Temperature -->
          <div>
            <div class="flex justify-between mb-2">
              <label class="text-sm font-medium flex items-center gap-2">
                Teplota (Temperature)
                <i class="pi pi-info-circle text-gray-400 text-xs cursor-help" v-tooltip="'Nejdůležitější parametr pro kontrolu kreativity. Hodnota 0 = vždy stejná odpověď, 2 = velmi náhodné.'"></i>
              </label>
              <span class="text-sm text-gray-500">{{ temperature ?? 'výchozí' }}</span>
            </div>
            <Slider v-model="temperatureSlider" :min="0" :max="2" :step="0.1" class="w-full" />
            <div class="flex justify-between text-xs text-gray-500 mt-1">
              <span>0 = Přesné, faktické</span>
              <span>1 = Vyvážené</span>
              <span>2 = Kreativní, náhodné</span>
            </div>
          </div>

          <Divider />

          <!-- Max Tokens -->
          <div>
            <label class="text-sm font-medium block mb-2">Max tokenů odpovědi</label>
            <InputNumber
              v-model="maxTokens"
              :min="1"
              :max="128000"
              placeholder="výchozí"
              class="w-full"
            />
            <p class="text-xs text-gray-500 mt-1">Maximální délka vygenerované odpovědi</p>
          </div>

          <Divider />

          <!-- Top P -->
          <div>
            <div class="flex justify-between mb-2">
              <label class="text-sm font-medium">Top P (Nucleus Sampling)</label>
              <span class="text-sm text-gray-500">{{ topP ?? 'výchozí' }}</span>
            </div>
            <Slider v-model="topPSlider" :min="0" :max="1" :step="0.05" class="w-full" />
            <p class="text-xs text-gray-500 mt-1">Používá se místo temperature pro kontrolu náhodnosti</p>
          </div>

          <!-- Local providers: Top K -->
          <div v-if="isLocalProvider">
            <Divider />
            <div>
              <label class="text-sm font-medium block mb-2">Top K</label>
              <InputNumber v-model="topK" :min="1" :max="100" placeholder="výchozí" class="w-full" />
              <p class="text-xs text-gray-500 mt-1">Počet nejvíce pravděpodobných tokenů k uvážení</p>
            </div>
          </div>

          <!-- OpenAI specific: Penalties -->
          <div v-if="isOpenAI">
            <Divider />
            <div class="grid grid-cols-2 gap-4">
              <div>
                <div class="flex justify-between mb-2">
                  <label class="text-sm font-medium">Frequency Penalty</label>
                  <span class="text-xs text-gray-500">{{ frequencyPenalty ?? '0' }}</span>
                </div>
                <Slider v-model="frequencyPenaltySlider" :min="-2" :max="2" :step="0.1" class="w-full" />
              </div>
              <div>
                <div class="flex justify-between mb-2">
                  <label class="text-sm font-medium">Presence Penalty</label>
                  <span class="text-xs text-gray-500">{{ presencePenalty ?? '0' }}</span>
                </div>
                <Slider v-model="presencePenaltySlider" :min="-2" :max="2" :step="0.1" class="w-full" />
              </div>
            </div>
          </div>

          <!-- Local providers: Repeat Penalty -->
          <div v-if="isLocalProvider">
            <div>
              <div class="flex justify-between mb-2">
                <label class="text-sm font-medium">Repeat Penalty</label>
                <span class="text-sm text-gray-500">{{ repeatPenalty ?? 'výchozí' }}</span>
              </div>
              <Slider v-model="repeatPenaltySlider" :min="0" :max="2" :step="0.1" class="w-full" />
            </div>
          </div>
          </div>
        </TabPanel>

        <!-- Features Tab -->
        <TabPanel value="features">
          <div class="space-y-4">
          <!-- Educational info box -->
          <div class="info-box info-box-purple">
            <div class="flex items-start gap-2">
              <i class="pi pi-sparkles info-box-icon mt-0.5" />
              <div>
                <p class="info-box-title">Pokročilé funkce LLM</p>
                <p class="info-box-text mt-1">
                  Moderní jazykové modely nabízí pokročilé funkce jako <strong>Thinking</strong> (model ukazuje svůj myšlenkový proces)
                  a <strong>Tools</strong> (model může volat externí nástroje). Tyto funkce významně zlepšují kvalitu odpovědí
                  pro složité úlohy, ale zvyšují dobu odezvy a náklady.
                </p>
              </div>
            </div>
          </div>

          <!-- Streaming -->
          <div class="feature-row">
            <div class="feature-row-text">
              <div class="feature-row-title flex items-center gap-2">
                Streaming
                <i class="pi pi-info-circle text-gray-400 text-xs cursor-help" v-tooltip="'Odpověď se zobrazuje postupně jak model generuje tokeny. Bez streamingu byste čekali na celou odpověď.'"></i>
              </div>
              <div class="feature-row-desc">Zobrazovat odpověď postupně jak se generuje</div>
            </div>
            <ToggleSwitch v-model="enableStreaming" />
          </div>

          <!-- Thinking -->
          <div class="feature-row flex-col !items-start space-y-3">
            <div class="flex items-center justify-between w-full">
              <div class="feature-row-text">
                <div class="feature-row-title flex items-center gap-2">
                  <i class="pi pi-lightbulb icon-purple"></i>
                  Přemýšlení (Thinking)
                </div>
                <div class="feature-row-desc">Povolit rozšířené uvažování u podporovaných modelů</div>
              </div>
              <ToggleSwitch v-model="enableThinking" />
            </div>
            <!-- Thinking Budget (shown when thinking is enabled for local providers) -->
            <div v-if="enableThinking && isLocalProvider" class="pt-2 border-t border-gray-200 dark:border-gray-700">
              <label class="text-sm text-gray-600 dark:text-gray-400 block mb-2">Úroveň přemýšlení</label>
              <div class="flex gap-2">
                <button
                  v-for="level in ['low', 'medium', 'high']"
                  :key="level"
                  @click="thinkingBudget = level"
                  class="flex-1 px-3 py-2 rounded-lg text-sm font-medium transition-colors"
                  :class="thinkingBudget === level
                    ? 'bg-purple-500 text-white'
                    : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'"
                >
                  {{ level === 'low' ? 'Nízká' : level === 'medium' ? 'Střední' : 'Vysoká' }}
                </button>
              </div>
              <div class="text-xs text-gray-500 mt-1">
                {{ thinkingBudget === 'low' ? 'Rychlejší odpovědi, kratší úvahy' : thinkingBudget === 'medium' ? 'Vyvážený poměr rychlosti a hloubky' : 'Hlubší analýza, delší doba zpracování' }}
              </div>
            </div>
          </div>

          <!-- Tools -->
          <div class="feature-row flex-col !items-start space-y-3">
            <div class="flex items-center justify-between w-full">
              <div class="feature-row-text">
                <div class="feature-row-title flex items-center gap-2">
                  <i class="pi pi-wrench icon-primary"></i>
                  Nástroje (Tools)
                </div>
                <div class="feature-row-desc">Povolit volání funkcí a nástrojů</div>
              </div>
              <ToggleSwitch v-model="enableTools" />
            </div>
            <!-- Max Tool Iterations (ReAct) - shown when tools enabled -->
            <div v-if="enableTools" class="w-full pt-2 border-t border-gray-200 dark:border-gray-700">
              <div class="flex items-center gap-2 mb-2">
                <i class="pi pi-sync text-purple-500 text-sm"></i>
                <label class="text-sm text-gray-600 dark:text-gray-400">Max iterací ReAct smyčky</label>
              </div>
              <div class="flex items-center gap-3">
                <InputNumber
                  v-model="maxToolIterations"
                  :min="1"
                  :max="50"
                  placeholder="10"
                  class="w-24"
                  :inputClass="'text-center'"
                />
                <span class="text-xs text-gray-500">výchozí: 10, max: 50</span>
              </div>
              <p class="text-xs text-gray-500 mt-2">
                ReAct (Reasoning and Acting) umožňuje modelu iterativně volat nástroje a zpracovávat výsledky.
                Vyšší limit = více iterací = komplexnější úlohy, ale delší doba zpracování.
              </p>
            </div>
          </div>

          <!-- Response Format (OpenAI) -->
          <div v-if="isOpenAI" class="p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
            <label class="font-medium block mb-2">Formát odpovědi</label>
            <Select
              v-model="responseFormat"
              :options="[
                { label: 'Text', value: 'text' },
                { label: 'JSON', value: 'json_object' },
              ]"
              optionLabel="label"
              optionValue="value"
              class="w-full"
            />
          </div>

        </div>
        </TabPanel>

        <!-- Context Tab -->
        <TabPanel value="context">
          <div class="space-y-5">
          <!-- Sliding Window Info Box -->
          <div class="info-box info-box-blue">
            <div class="flex items-start gap-2">
              <i class="pi pi-info-circle info-box-icon mt-0.5" />
              <div>
                <p class="info-box-text">
                  <strong class="info-box-title">Posuvné okno (Sliding Window)</strong> omezuje počet zpráv odesílaných s každým požadavkem.
                  Starší zprávy se automaticky odříznou, což šetří tokeny a snižuje náklady.
                </p>
              </div>
            </div>
          </div>

          <!-- Max History (Sliding Window) -->
          <div class="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
            <div class="flex items-center justify-between mb-3">
              <div>
                <label class="text-sm font-medium">Posuvné okno historie</label>
                <p class="text-xs text-gray-500">Ponechat maximálně N posledních zpráv</p>
              </div>
              <div class="text-right">
                <span class="text-lg font-mono font-bold">{{ maxHistoryLength ?? '∞' }}</span>
                <span class="text-xs text-gray-500 ml-1">zpráv</span>
              </div>
            </div>
            <Slider
              v-model="maxHistorySlider"
              :min="4"
              :max="100"
              :step="2"
              class="w-full"
            />
            <div class="flex justify-between text-xs text-gray-500 mt-1">
              <span>4 (úsporné)</span>
              <span>Klikněte pro reset</span>
              <span>100 (plná historie)</span>
            </div>
            <Button
              v-if="maxHistoryLength"
              label="Vypnout limit"
              icon="pi pi-times"
              size="small"
              severity="secondary"
              text
              @click="maxHistoryLength = null"
              class="mt-2"
            />
          </div>

          <Divider />

          <!-- Context Window Size -->
          <div>
            <label class="text-sm font-medium block mb-2">Velikost kontextového okna (tokeny)</label>
            <InputNumber
              v-model="contextLength"
              :min="1024"
              :max="1000000"
              :step="1024"
              placeholder="výchozí dle modelu"
              class="w-full"
            />
            <p class="text-xs text-gray-500 mt-1">Maximální počet tokenů v kontextu (systém + historie + odpověď)</p>
          </div>

          <!-- Local provider specific (Ollama / llama.cpp) -->
          <div v-if="isLocalProvider">
            <Divider />
            <div class="text-sm font-medium mb-3 flex items-center gap-2">
              <i class="pi pi-server text-blue-500" />
              {{ isLlamaCpp ? 'Nastavení llama.cpp' : 'Nastavení Ollama' }}
            </div>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="text-sm font-medium block mb-2">num_ctx</label>
                <InputNumber v-model="numCtx" :min="512" :max="131072" placeholder="výchozí" class="w-full" />
                <p class="text-xs text-gray-500 mt-1">Velikost kontextového okna</p>
              </div>
              <div>
                <label class="text-sm font-medium block mb-2">num_predict</label>
                <InputNumber v-model="numPredict" :min="1" :max="10000" placeholder="výchozí" class="w-full" />
                <p class="text-xs text-gray-500 mt-1">Max tokenů k predikci</p>
              </div>
              <div>
                <label class="text-sm font-medium block mb-2">
                  <i class="pi pi-sync text-purple-500 mr-1"></i>
                  Seed
                </label>
                <InputNumber v-model="seed" :min="0" placeholder="náhodný" class="w-full" />
                <p class="text-xs text-gray-500 mt-1">Pro reprodukovatelné výsledky</p>
              </div>
            </div>

            <!-- Grammar (GBNF) -->
            <div class="mt-4">
              <label class="text-sm font-medium block mb-2">
                <i class="pi pi-code text-blue-500 mr-1"></i>
                Grammar (GBNF)
              </label>
              <Textarea
                v-model="grammar"
                placeholder='root ::= object&#10;object ::= "{" ws "name" ws ":" ws string "}"&#10;string ::= "\"" [a-zA-Z]+ "\""&#10;ws ::= " "?'
                :rows="3"
                class="w-full font-mono text-xs"
              />
              <p class="text-xs text-gray-500 mt-1">
                GBNF gramatika pro strukturovaný výstup.
                <a href="https://github.com/ggerganov/llama.cpp/blob/master/grammars/README.md" target="_blank" class="text-blue-500 hover:underline">Dokumentace</a>
              </p>
            </div>
          </div>

          <Divider />

          <!-- Stop Sequences -->
          <div>
            <label class="text-sm font-medium block mb-2">Stop sekvence</label>
            <InputText
              v-model="stopSequences"
              placeholder="např: ###, END, ..."
              class="w-full"
            />
            <p class="text-xs text-gray-500 mt-1">Texty oddělené čárkou, které ukončí generování</p>
          </div>

          <!-- Tips -->
          <div class="info-box info-box-yellow">
            <div class="info-box-title mb-1">
              <i class="pi pi-lightbulb mr-1" /> Tipy pro úsporu tokenů
            </div>
            <ul class="info-box-text space-y-1 ml-4 list-disc text-xs">
              <li>Nastavte posuvné okno na 20-30 zpráv pro většinu konverzací</li>
              <li>Používejte tlačítko "Spravovat kontext" pro ruční kompaktování</li>
              <li>Claude podporuje prompt caching - opakovaný kontext stojí jen 10%</li>
            </ul>
          </div>
          </div>
        </TabPanel>
        </TabPanels>
      </Tabs>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <Button
          label="Zrušit"
          severity="secondary"
          @click="emit('update:visible', false)"
        />
        <Button
          :label="isEditMode ? 'Uložit změny' : 'Vytvořit chat'"
          :icon="isEditMode ? 'pi pi-save' : 'pi pi-plus'"
          @click="handleSubmit"
          :disabled="!selectedProvider || !selectedModel"
        />
      </div>
    </template>
  </Dialog>
</template>

<style scoped>
/* Wrapper with fixed height */
.settings-tabs-wrapper {
  height: 37.5rem; /* 600px */
  display: flex;
  flex-direction: column;
}

.settings-tabs-wrapper :deep(.p-tabs) {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.settings-tabs-wrapper :deep(.p-tablist) {
  flex-shrink: 0;
}

.settings-tabs-wrapper :deep(.p-tabpanels) {
  flex: 1;
  overflow: hidden;
}

.settings-tabs-wrapper :deep(.p-tabpanel) {
  height: 100%;
  overflow-y: auto;
  overflow-x: hidden;
  min-height: unset !important;
}

/* Custom scrollbar */
.settings-tabs-wrapper :deep(.p-tabpanel)::-webkit-scrollbar {
  width: 0.375rem; /* 6px */
}

.settings-tabs-wrapper :deep(.p-tabpanel)::-webkit-scrollbar-track {
  @apply bg-gray-100 dark:bg-gray-800 rounded-full;
}

.settings-tabs-wrapper :deep(.p-tabpanel)::-webkit-scrollbar-thumb {
  @apply bg-gray-300 dark:bg-gray-600 rounded-full;
}

.settings-tabs-wrapper :deep(.p-tabpanel)::-webkit-scrollbar-thumb:hover {
  @apply bg-gray-400 dark:bg-gray-500;
}
</style>
