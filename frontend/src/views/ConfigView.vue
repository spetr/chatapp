<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Sidebar from '@/components/Sidebar.vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Textarea from 'primevue/textarea'
import InputNumber from 'primevue/inputnumber'
import Checkbox from 'primevue/checkbox'
import Accordion from 'primevue/accordion'
import AccordionPanel from 'primevue/accordionpanel'
import AccordionHeader from 'primevue/accordionheader'
import AccordionContent from 'primevue/accordioncontent'
import ProgressSpinner from 'primevue/progressspinner'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Slider from 'primevue/slider'

const router = useRouter()
const toast = useToast()

interface ProviderConfig {
  type: string
  api_key: string
  base_url: string
  has_key: boolean
}

interface PromptConfig {
  name: string
  description: string
  content: string
}

interface MCPServer {
  name: string
  command: string
  args: string[]
  env: Record<string, string>
  enabled: boolean
}

interface ContextConfig {
  max_messages: number
  max_tokens: number
  truncate_long_msgs: boolean
  max_msg_length: number
}

interface Config {
  providers: Record<string, ProviderConfig>
  prompts: Record<string, PromptConfig>
  mcp: { servers: MCPServer[] }
  context: ContextConfig
}

interface ConfigPath {
  path: string
  is_set: boolean
  is_memory: boolean
}

const isLoading = ref(true)
const isSaving = ref(false)
const config = ref<Config | null>(null)
const configPath = ref<ConfigPath | null>(null)
const backendAvailable = ref(true)

// Editable API keys (separate from display)
const editableKeys = ref<Record<string, string>>({})

// GPU configuration for local providers (Ollama/llama.cpp)
interface GPUSpec {
  name: string
  tdp: number
  prompt_tok_per_sec: number
  gen_tok_per_sec: number
  vram: number
}

interface OllamaConfig {
  gpu_id: string
  pue: number
  electricity_rate: number
}

const gpuOptions = ref<Record<string, GPUSpec>>({})
const ollamaConfig = ref<OllamaConfig>({
  gpu_id: 'rtx-4090',
  pue: 1.1,
  electricity_rate: 0.12,
})
const isLoadingGPU = ref(false)

// Computed price estimate
const estimatedPricing = computed(() => {
  const gpu = gpuOptions.value[ollamaConfig.value.gpu_id]
  if (!gpu) return null

  const totalWatts = gpu.tdp * ollamaConfig.value.pue
  const costPerHour = (totalWatts / 1000) * ollamaConfig.value.electricity_rate
  const promptTokPerHour = gpu.prompt_tok_per_sec * 3600
  const genTokPerHour = gpu.gen_tok_per_sec * 3600

  const inputPer1M = (costPerHour / promptTokPerHour) * 1_000_000
  const outputPer1M = (costPerHour / genTokPerHour) * 1_000_000

  return {
    input: inputPer1M.toFixed(4),
    output: outputPer1M.toFixed(4),
    costPerHour: costPerHour.toFixed(3),
  }
})

// Provider display names
const providerNames: Record<string, string> = {
  claude: 'Claude (Anthropic)',
  openai: 'OpenAI',
  ollama: 'Ollama',
}

const providerDescriptions: Record<string, string> = {
  anthropic: 'Modely Claude od Anthropic. Vynikají v komplexním uvažování a programování.',
  openai: 'Modely GPT od OpenAI. Průmyslový standard s voláním funkcí.',
  ollama: 'Lokální modely přes Ollama. Běží na vašem počítači bez nákladů.',
}

// Default config when backend is not available
function getDefaultConfig(): Config {
  return {
    providers: {
      anthropic: {
        type: 'anthropic',
        api_key: '',
        base_url: '',
        has_key: false,
      },
      openai: {
        type: 'openai',
        api_key: '',
        base_url: '',
        has_key: false,
      },
      ollama: {
        type: 'ollama',
        api_key: '',
        base_url: 'http://localhost:11434',
        has_key: false,
      },
      llamacpp: {
        type: 'llamacpp',
        api_key: '',
        base_url: 'http://localhost:8080',
        has_key: false,
      },
    },
    prompts: {
      default: {
        name: 'Výchozí',
        description: 'Výchozí asistent',
        content: 'Jsi užitečný asistent.',
      },
      coder: {
        name: 'Programátor',
        description: 'Asistent pro programování',
        content: 'Jsi zkušený programátor. Pomáháš s kódem, vysvětluješ koncepty a poskytuj fungující příklady.',
      },
    },
    mcp: { servers: [] },
    context: {
      max_messages: 50,
      max_tokens: 100000,
      truncate_long_msgs: true,
      max_msg_length: 4000,
    },
  }
}

onMounted(async () => {
  await Promise.all([loadConfig(), loadGPUConfig()])
})

async function loadGPUConfig() {
  isLoadingGPU.value = true
  try {
    const [gpuResp, configResp] = await Promise.all([
      fetch('/api/ollama/gpus'),
      fetch('/api/ollama/config'),
    ])

    if (gpuResp.ok) {
      const data = await gpuResp.json()
      gpuOptions.value = data.gpus || {}
    }

    if (configResp.ok) {
      const data = await configResp.json()
      ollamaConfig.value = {
        gpu_id: data.gpu_id || 'rtx-4090',
        pue: data.pue || 1.1,
        electricity_rate: data.electricity_rate || 0.12,
      }
    }
  } catch (error) {
    console.error('Failed to load GPU config:', error)
  } finally {
    isLoadingGPU.value = false
  }
}

async function saveGPUConfig() {
  try {
    const response = await fetch('/api/ollama/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(ollamaConfig.value),
    })

    if (response.ok) {
      toast.add({
        severity: 'success',
        summary: 'Uloženo',
        detail: 'Nastavení GPU bylo uloženo',
        life: 2000,
      })
    }
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: 'Chyba',
      detail: 'Nepodařilo se uložit nastavení GPU',
      life: 3000,
    })
  }
}

async function loadConfig() {
  isLoading.value = true
  try {
    const [configResp, pathResp] = await Promise.all([
      fetch('/api/config'),
      fetch('/api/config/path'),
    ])

    if (configResp.ok) {
      config.value = await configResp.json()
      backendAvailable.value = true
    } else {
      // Use default config if backend returns error
      config.value = getDefaultConfig()
      backendAvailable.value = false
    }

    // Initialize editable keys as empty (user must re-enter to change)
    editableKeys.value = {}
    for (const name of Object.keys(config.value?.providers || {})) {
      editableKeys.value[name] = ''
    }

    if (pathResp.ok) {
      configPath.value = await pathResp.json()
    }
  } catch (error) {
    console.error('Failed to load config:', error)
    // Use default config when backend is not available
    config.value = getDefaultConfig()
    backendAvailable.value = false
    editableKeys.value = {}
    for (const name of Object.keys(config.value?.providers || {})) {
      editableKeys.value[name] = ''
    }
  } finally {
    isLoading.value = false
  }
}

async function saveConfig() {
  if (!config.value) return

  if (!backendAvailable.value) {
    toast.add({
      severity: 'error',
      summary: 'Backend nedostupný',
      detail: 'Pro uložení konfigurace je potřeba spustit backend server',
      life: 5000,
    })
    return
  }

  isSaving.value = true
  try {
    // Build update request - include all providers with their settings
    const providers: Record<string, { api_key?: string; base_url?: string }> = {}

    for (const [name, prov] of Object.entries(config.value.providers)) {
      const update: { api_key?: string; base_url?: string } = {}

      // Include new API key if entered
      const newKey = editableKeys.value[name]?.trim()
      if (newKey) {
        update.api_key = newKey
      }

      // Always include base_url if set
      if (prov.base_url) {
        update.base_url = prov.base_url
      }

      // Only add provider to update if there's something to update
      if (Object.keys(update).length > 0) {
        providers[name] = update
      }
    }

    const response = await fetch('/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        providers,
        prompts: config.value.prompts,
        mcp: config.value.mcp,
        context: config.value.context,
      }),
    })

    if (response.ok) {
      toast.add({
        severity: 'success',
        summary: 'Uloženo',
        detail: 'Konfigurace byla uložena na disk',
        life: 3000,
      })
      // Clear editable keys after successful save
      for (const name of Object.keys(editableKeys.value)) {
        editableKeys.value[name] = ''
      }
      // Reload to get updated masked keys
      await loadConfig()
    } else {
      const errorData = await response.json().catch(() => ({}))
      throw new Error(errorData.error || 'Save failed')
    }
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: 'Chyba',
      detail: `Nepodařilo se uložit konfiguraci: ${(error as Error).message}`,
      life: 5000,
    })
  } finally {
    isSaving.value = false
  }
}

function addMCPServer() {
  if (!config.value) return
  config.value.mcp.servers.push({
    name: '',
    command: '',
    args: [],
    env: {},
    enabled: true,
  })
}

function removeMCPServer(index: number) {
  if (!config.value) return
  config.value.mcp.servers.splice(index, 1)
}

function addPrompt() {
  if (!config.value) return
  const id = `custom_${Date.now()}`
  config.value.prompts[id] = {
    name: 'Nový prompt',
    description: '',
    content: '',
  }
}

function removePrompt(id: string) {
  if (!config.value) return
  delete config.value.prompts[id]
}

const sortedProviders = computed(() => {
  if (!config.value) return []
  return Object.entries(config.value.providers).sort(([a], [b]) => a.localeCompare(b))
})

const sortedPrompts = computed(() => {
  if (!config.value) return []
  return Object.entries(config.value.prompts).sort(([a], [b]) => a.localeCompare(b))
})

</script>

<template>
  <div class="h-screen flex">
    <Sidebar />

    <div class="flex-1 flex flex-col overflow-hidden">
      <!-- Header -->
      <header class="h-14 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between px-4 bg-white dark:bg-gray-900">
        <div class="flex items-center gap-3">
          <Button
            icon="pi pi-arrow-left"
            @click="router.push('/')"
            text
            rounded
            severity="secondary"
          />
          <h1 class="font-semibold">Konfigurace</h1>
        </div>
      </header>

      <div class="flex-1 overflow-y-auto p-6">
        <!-- Loading -->
        <div v-if="isLoading" class="flex items-center justify-center h-full">
          <ProgressSpinner />
        </div>

        <div v-else-if="config" class="max-w-4xl mx-auto space-y-6">
          <!-- Backend not available warning -->
          <Message v-if="!backendAvailable" severity="error" :closable="false">
            <div class="flex items-center gap-2">
              <i class="pi pi-exclamation-circle"></i>
              <div>
                <strong>Backend není dostupný</strong>
                <p class="text-sm mt-1">
                  Pro uložení konfigurace spusťte backend server:
                  <code class="bg-gray-100 dark:bg-gray-700 px-2 py-0.5 rounded ml-1">make run</code>
                  nebo
                  <code class="bg-gray-100 dark:bg-gray-700 px-2 py-0.5 rounded ml-1">./bin/chatapp-server</code>
                </p>
              </div>
            </div>
          </Message>

          <!-- Config Path Info -->
          <Message v-else-if="configPath?.is_memory" severity="info" :closable="false">
            <div class="flex items-center gap-2">
              <i class="pi pi-info-circle"></i>
              <span>
                Konfigurační soubor zatím neexistuje. Po kliknutí na "Uložit změny" se automaticky vytvoří <code>config.json</code>.
              </span>
            </div>
          </Message>
          <Message v-else-if="configPath?.path" severity="info" :closable="false">
            <div class="flex items-center gap-2">
              <i class="pi pi-file"></i>
              <span>Konfigurační soubor: <code>{{ configPath.path }}</code></span>
            </div>
          </Message>

          <!-- Providers -->
          <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
            <h2 class="text-lg font-semibold mb-4 flex items-center gap-2">
              <i class="pi pi-cloud"></i>
              Poskytovatelé AI
            </h2>

            <Accordion :multiple="true">
              <AccordionPanel
                v-for="[name, prov] in sortedProviders"
                :key="name"
                :value="name"
              >
                <AccordionHeader>
                  <div class="flex items-center gap-3">
                    <span :class="prov.has_key ? 'text-green-500' : 'text-gray-400'">
                      <i :class="prov.has_key ? 'pi pi-check-circle' : 'pi pi-circle'"></i>
                    </span>
                    <span class="font-medium">{{ providerNames[name] || name }}</span>
                    <span v-if="prov.has_key || prov.type === 'ollama'" class="text-xs bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300 px-2 py-0.5 rounded">
                      {{ prov.type === 'ollama' ? 'Lokální' : 'Nakonfigurováno' }}
                    </span>
                    <span v-else class="text-xs bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300 px-2 py-0.5 rounded">
                      Vyžaduje API klíč
                    </span>
                  </div>
                </AccordionHeader>
                <AccordionContent>
                  <div class="space-y-4 pt-2">
                    <p class="text-sm text-gray-600 dark:text-gray-400">
                      {{ providerDescriptions[prov.type] || prov.type }}
                    </p>

                    <!-- API Key (not for Ollama) -->
                    <div v-if="prov.type !== 'ollama'">
                      <label class="block text-sm font-medium mb-1">API klíč</label>
                      <div class="flex gap-2">
                        <Password
                          v-model="editableKeys[name]"
                          :placeholder="prov.has_key ? prov.api_key : 'Zadejte API klíč...'"
                          :feedback="false"
                          toggleMask
                          class="flex-1"
                          inputClass="w-full"
                        />
                      </div>
                      <p class="text-xs text-gray-500 mt-1">
                        <template v-if="prov.has_key">
                          Aktuální klíč: {{ prov.api_key }}. Pro změnu zadejte nový.
                        </template>
                        <template v-else>
                          Zadejte API klíč pro aktivaci tohoto poskytovatele.
                        </template>
                      </p>
                    </div>

                    <!-- Base URL (for OpenAI compatible) -->
                    <div v-if="prov.type === 'openai' || prov.type === 'ollama'">
                      <label class="block text-sm font-medium mb-1">Base URL (volitelné)</label>
                      <InputText
                        v-model="prov.base_url"
                        placeholder="https://api.openai.com/v1"
                        class="w-full"
                      />
                      <p class="text-xs text-gray-500 mt-1">
                        Pro OpenAI kompatibilní API (Azure, lokální servery, atd.)
                      </p>
                    </div>

                  </div>
                </AccordionContent>
              </AccordionPanel>
            </Accordion>
          </div>

          <!-- GPU Configuration for Local Providers -->
          <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
            <h2 class="text-lg font-semibold mb-4 flex items-center gap-2">
              <i class="pi pi-desktop text-green-500"></i>
              Nastavení GPU (Ollama / llama.cpp)
            </h2>

            <div class="space-y-4">
              <!-- Info box -->
              <div class="p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-200 dark:border-blue-800 text-sm">
                <div class="flex items-start gap-2">
                  <i class="pi pi-info-circle text-blue-500 mt-0.5"></i>
                  <div>
                    <p class="text-blue-800 dark:text-blue-200">
                      Pro lokální modely se cena počítá z nákladů na elektřinu.
                      Výběrem GPU a zadáním ceny elektřiny získáte přibližný odhad nákladů.
                    </p>
                  </div>
                </div>
              </div>

              <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <!-- GPU Selection -->
                <div>
                  <label class="block text-sm font-medium mb-2">GPU akcelerátor</label>
                  <Select
                    v-model="ollamaConfig.gpu_id"
                    :options="Object.entries(gpuOptions).map(([id, gpu]) => ({ id, ...gpu }))"
                    optionValue="id"
                    optionLabel="name"
                    placeholder="Vyberte GPU"
                    class="w-full"
                    @change="saveGPUConfig"
                  >
                    <template #option="{ option }">
                      <div class="flex justify-between items-center w-full">
                        <span>{{ option.name }}</span>
                        <span class="text-xs text-gray-500">
                          {{ option.tdp }}W / {{ option.vram }}GB
                        </span>
                      </div>
                    </template>
                  </Select>
                  <p class="text-xs text-gray-500 mt-1">
                    Vyberte GPU, které používáte pro inference
                  </p>
                </div>

                <!-- Electricity Rate -->
                <div>
                  <label class="block text-sm font-medium mb-2">
                    Cena elektřiny ($/kWh)
                  </label>
                  <div class="flex items-center gap-3">
                    <Slider
                      v-model="ollamaConfig.electricity_rate"
                      :min="0.05"
                      :max="0.50"
                      :step="0.01"
                      class="flex-1"
                      @slideend="saveGPUConfig"
                    />
                    <span class="font-mono text-sm w-16 text-right">
                      ${{ ollamaConfig.electricity_rate.toFixed(2) }}
                    </span>
                  </div>
                  <p class="text-xs text-gray-500 mt-1">
                    Průměrná cena elektřiny v ČR: ~$0.25/kWh
                  </p>
                </div>

                <!-- PUE -->
                <div>
                  <label class="block text-sm font-medium mb-2">
                    PUE (Power Usage Effectiveness)
                  </label>
                  <div class="flex items-center gap-3">
                    <Slider
                      v-model="ollamaConfig.pue"
                      :min="1.0"
                      :max="2.0"
                      :step="0.1"
                      class="flex-1"
                      @slideend="saveGPUConfig"
                    />
                    <span class="font-mono text-sm w-12 text-right">
                      {{ ollamaConfig.pue.toFixed(1) }}
                    </span>
                  </div>
                  <p class="text-xs text-gray-500 mt-1">
                    1.0 = žádné chlazení, 1.5-2.0 = datové centrum
                  </p>
                </div>

                <!-- Estimated Pricing -->
                <div v-if="estimatedPricing" class="p-3 bg-green-50 dark:bg-green-900/20 rounded-lg border border-green-200 dark:border-green-800">
                  <div class="text-sm font-medium text-green-800 dark:text-green-200 mb-2">
                    <i class="pi pi-calculator mr-1"></i>
                    Odhadovaná cena
                  </div>
                  <div class="grid grid-cols-2 gap-2 text-sm">
                    <div>
                      <span class="text-gray-600 dark:text-gray-400">Vstup:</span>
                      <span class="font-mono ml-1">${{ estimatedPricing.input }}/1M</span>
                    </div>
                    <div>
                      <span class="text-gray-600 dark:text-gray-400">Výstup:</span>
                      <span class="font-mono ml-1">${{ estimatedPricing.output }}/1M</span>
                    </div>
                  </div>
                  <div class="mt-2 pt-2 border-t border-green-200 dark:border-green-700 text-xs text-gray-500">
                    Cena za hodinu provozu: ${{ estimatedPricing.costPerHour }}
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Context Settings -->
          <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
            <h2 class="text-lg font-semibold mb-4 flex items-center gap-2">
              <i class="pi pi-database"></i>
              Nastavení kontextu
            </h2>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm font-medium mb-1">Max. zpráv v kontextu</label>
                <InputNumber
                  v-model="config.context.max_messages"
                  :min="0"
                  :max="1000"
                  class="w-full"
                />
                <p class="text-xs text-gray-500 mt-1">0 = neomezeno</p>
              </div>

              <div>
                <label class="block text-sm font-medium mb-1">Max. vstupních tokenů</label>
                <InputNumber
                  v-model="config.context.max_tokens"
                  :min="0"
                  :step="1000"
                  class="w-full"
                />
                <p class="text-xs text-gray-500 mt-1">0 = neomezeno</p>
              </div>

              <div>
                <label class="block text-sm font-medium mb-1">Max. délka zprávy (znaky)</label>
                <InputNumber
                  v-model="config.context.max_msg_length"
                  :min="0"
                  :step="1000"
                  class="w-full"
                />
              </div>

              <div class="flex items-center gap-2">
                <Checkbox
                  v-model="config.context.truncate_long_msgs"
                  :binary="true"
                  inputId="truncate"
                />
                <label for="truncate" class="text-sm">Zkrátit dlouhé zprávy</label>
              </div>
            </div>
          </div>

          <!-- MCP Servers -->
          <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
            <div class="flex items-center justify-between mb-4">
              <h2 class="text-lg font-semibold flex items-center gap-2">
                <i class="pi pi-box"></i>
                MCP servery (nástroje)
              </h2>
              <Button
                label="Přidat server"
                icon="pi pi-plus"
                size="small"
                @click="addMCPServer"
              />
            </div>

            <div v-if="config.mcp.servers.length === 0" class="text-center text-gray-500 py-8">
              <i class="pi pi-inbox text-3xl mb-2"></i>
              <p>Žádné MCP servery nakonfigurovány</p>
              <p class="text-sm">MCP servery poskytují nástroje jako souborový systém, databáze, atd.</p>
            </div>

            <div v-else class="space-y-4">
              <div
                v-for="(server, index) in config.mcp.servers"
                :key="index"
                class="border border-gray-200 dark:border-gray-600 rounded-lg p-4"
              >
                <div class="flex items-start justify-between gap-4">
                  <div class="flex-1 space-y-3">
                    <div class="flex items-center gap-4">
                      <Checkbox
                        v-model="server.enabled"
                        :binary="true"
                        :inputId="`mcp-${index}`"
                      />
                      <div class="flex-1">
                        <label class="block text-sm font-medium mb-1">Název</label>
                        <InputText
                          v-model="server.name"
                          placeholder="filesystem"
                          class="w-full"
                        />
                      </div>
                    </div>

                    <div>
                      <label class="block text-sm font-medium mb-1">Příkaz</label>
                      <InputText
                        v-model="server.command"
                        placeholder="npx"
                        class="w-full"
                      />
                    </div>

                    <div>
                      <label class="block text-sm font-medium mb-1">Argumenty (jeden na řádek)</label>
                      <Textarea
                        :modelValue="server.args.join('\n')"
                        @update:modelValue="server.args = ($event as string).split('\n').filter(a => a.trim())"
                        rows="3"
                        placeholder="-y&#10;@anthropic/mcp-server-filesystem&#10;/path/to/dir"
                        class="w-full font-mono text-sm"
                      />
                    </div>
                  </div>

                  <Button
                    icon="pi pi-trash"
                    severity="danger"
                    text
                    rounded
                    @click="removeMCPServer(index)"
                  />
                </div>
              </div>
            </div>
          </div>

          <!-- System Prompts -->
          <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
            <div class="flex items-center justify-between mb-4">
              <h2 class="text-lg font-semibold flex items-center gap-2">
                <i class="pi pi-comment"></i>
                Šablony systémových promptů
              </h2>
              <Button
                label="Přidat šablonu"
                icon="pi pi-plus"
                size="small"
                @click="addPrompt"
              />
            </div>

            <div class="space-y-4">
              <div
                v-for="[id, prompt] in sortedPrompts"
                :key="id"
                class="border border-gray-200 dark:border-gray-600 rounded-lg p-4"
              >
                <div class="flex items-start justify-between gap-4">
                  <div class="flex-1 space-y-3">
                    <div class="grid grid-cols-2 gap-4">
                      <div>
                        <label class="block text-sm font-medium mb-1">Název</label>
                        <InputText
                          v-model="prompt.name"
                          class="w-full"
                        />
                      </div>
                      <div>
                        <label class="block text-sm font-medium mb-1">Popis</label>
                        <InputText
                          v-model="prompt.description"
                          class="w-full"
                        />
                      </div>
                    </div>

                    <div>
                      <label class="block text-sm font-medium mb-1">Obsah</label>
                      <Textarea
                        v-model="prompt.content"
                        rows="4"
                        class="w-full"
                      />
                    </div>
                  </div>

                  <Button
                    v-if="id !== 'default'"
                    icon="pi pi-trash"
                    severity="danger"
                    text
                    rounded
                    @click="removePrompt(id)"
                  />
                </div>
              </div>
            </div>
          </div>

          <!-- Save Button (bottom) -->
          <div class="flex justify-end pb-6">
            <Button
              label="Uložit změny"
              icon="pi pi-save"
              @click="saveConfig"
              :loading="isSaving"
              size="large"
            />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
