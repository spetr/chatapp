<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useChatStore } from '@/stores/chat'
import { useThemeStore } from '@/stores/theme'
import ProviderIcon from '@/components/ProviderIcon.vue'

const emit = defineEmits<{
  newChat: []
}>()

const router = useRouter()
const chatStore = useChatStore()
const themeStore = useThemeStore()

// Group conversations by date
const groupedConversations = computed(() => {
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const yesterday = new Date(today.getTime() - 24 * 60 * 60 * 1000)
  const weekAgo = new Date(today.getTime() - 7 * 24 * 60 * 60 * 1000)

  const groups: { label: string; conversations: typeof chatStore.conversations }[] = [
    { label: 'Dnes', conversations: [] },
    { label: 'Včera', conversations: [] },
    { label: 'Tento týden', conversations: [] },
    { label: 'Starší', conversations: [] },
  ]

  chatStore.conversations.forEach((conv) => {
    const date = new Date(conv.updated_at)
    if (date >= today) {
      groups[0].conversations.push(conv)
    } else if (date >= yesterday) {
      groups[1].conversations.push(conv)
    } else if (date >= weekAgo) {
      groups[2].conversations.push(conv)
    } else {
      groups[3].conversations.push(conv)
    }
  })

  return groups.filter((g) => g.conversations.length > 0)
})

const selectedConversation = computed({
  get: () => chatStore.currentConversation?.id,
  set: (id) => {
    if (id) {
      router.push(`/chat/${id}`)
    }
  },
})

function handleNewChat() {
  chatStore.clearCurrentConversation()
  emit('newChat')
}

async function handleDeleteConversation(id: string, event: Event) {
  event.stopPropagation()
  if (confirm('Smazat tuto konverzaci?')) {
    await chatStore.deleteConversation(id)
    if (chatStore.currentConversation?.id === id) {
      router.push('/')
    }
  }
}

function formatTime(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())

  if (date >= today) {
    return date.toLocaleTimeString('cs-CZ', { hour: '2-digit', minute: '2-digit' })
  }
  return date.toLocaleDateString('cs-CZ', { day: 'numeric', month: 'short' })
}
</script>

<template>
  <aside class="w-72 shrink-0 h-full flex flex-col bg-gradient-to-b from-gray-50 to-gray-100 dark:from-gray-900 dark:to-gray-800 border-r border-gray-200 dark:border-gray-700">
    <!-- Header -->
    <div class="p-4">
      <!-- Logo & Theme Toggle -->
      <div class="flex items-center justify-between mb-4">
        <div class="flex items-center gap-2">
          <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center shadow-lg">
            <i class="pi pi-comments text-white text-sm"></i>
          </div>
          <div>
            <h1 class="font-bold text-gray-900 dark:text-white">ChatApp</h1>
            <p class="text-2xs text-gray-500 dark:text-gray-400 -mt-0.5">Tech Demo</p>
          </div>
        </div>
        <button
          @click="themeStore.toggle()"
          class="w-8 h-8 rounded-lg bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 flex items-center justify-center hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors shadow-sm"
          v-tooltip="themeStore.isDark ? 'Přepnout na světlý režim' : 'Přepnout na tmavý režim'"
        >
          <i :class="['pi text-sm', themeStore.isDark ? 'pi-sun text-yellow-500' : 'pi-moon text-gray-600']"></i>
        </button>
      </div>

      <!-- New Chat Button -->
      <button
        @click="handleNewChat"
        class="w-full py-2.5 px-4 rounded-xl bg-gradient-to-r from-blue-500 to-blue-600 hover:from-blue-600 hover:to-blue-700 text-white font-medium flex items-center justify-center gap-2 shadow-md hover:shadow-lg transition-all active:scale-[0.98]"
      >
        <i class="pi pi-plus text-sm"></i>
        <span>Nová konverzace</span>
      </button>
    </div>

    <!-- Conversations List -->
    <div class="flex-1 overflow-y-auto px-2 pb-2">
      <!-- Empty State -->
      <div v-if="chatStore.conversations.length === 0" class="flex flex-col items-center justify-center h-full text-center px-4">
        <div class="w-16 h-16 rounded-2xl bg-gray-200 dark:bg-gray-700 flex items-center justify-center mb-3">
          <i class="pi pi-inbox text-2xl text-gray-400"></i>
        </div>
        <p class="text-gray-500 dark:text-gray-400 text-sm">Zatím žádné konverzace</p>
        <p class="text-gray-400 dark:text-gray-500 text-xs mt-1">Začněte novou konverzaci</p>
      </div>

      <!-- Grouped Conversations -->
      <div v-else class="space-y-4">
        <div v-for="group in groupedConversations" :key="group.label">
          <!-- Group Label -->
          <div class="px-3 py-1.5 text-2xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">
            {{ group.label }}
          </div>

          <!-- Conversations -->
          <div class="space-y-1">
            <div
              v-for="conv in group.conversations"
              :key="conv.id"
              @click="selectedConversation = conv.id"
              :class="[
                'group relative p-3 rounded-xl cursor-pointer transition-all duration-200',
                selectedConversation === conv.id
                  ? 'bg-white dark:bg-gray-800 shadow-md border border-blue-200 dark:border-blue-800'
                  : 'hover:bg-white/60 dark:hover:bg-gray-800/60 border border-transparent',
              ]"
            >
              <!-- Selection Indicator -->
              <div
                v-if="selectedConversation === conv.id"
                class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-8 bg-blue-500 rounded-r-full"
              ></div>

              <div class="flex items-start gap-3">
                <!-- Provider Icon -->
                <div
                  :class="[
                    'w-9 h-9 rounded-lg flex items-center justify-center flex-shrink-0',
                    selectedConversation === conv.id
                      ? 'bg-blue-100 dark:bg-blue-900/50'
                      : 'bg-gray-100 dark:bg-gray-700/50',
                  ]"
                >
                  <ProviderIcon :provider="conv.provider" :size="18" />
                </div>

                <!-- Content -->
                <div class="flex-1 min-w-0">
                  <p class="font-medium text-gray-900 dark:text-white truncate text-sm">
                    {{ conv.title || 'Bez názvu' }}
                  </p>
                  <div class="flex items-center gap-2 mt-0.5">
                    <span class="text-2xs text-gray-500 dark:text-gray-400 truncate">
                      {{ conv.model.split('/').pop()?.split(':')[0] }}
                    </span>
                    <span class="text-gray-300 dark:text-gray-600">·</span>
                    <span class="text-2xs text-gray-400 dark:text-gray-500">
                      {{ formatTime(conv.updated_at) }}
                    </span>
                  </div>
                </div>

                <!-- Delete Button -->
                <button
                  @click="handleDeleteConversation(conv.id, $event)"
                  class="opacity-0 group-hover:opacity-100 w-7 h-7 rounded-lg flex items-center justify-center hover:bg-red-100 dark:hover:bg-red-900/30 text-gray-400 hover:text-red-500 transition-all"
                  v-tooltip="'Smazat konverzaci'"
                >
                  <i class="pi pi-trash text-xs"></i>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Footer -->
    <div class="p-3 border-t border-gray-200 dark:border-gray-700 bg-white/50 dark:bg-gray-800/50 backdrop-blur-sm">
      <router-link to="/config">
        <button class="w-full py-2 px-3 rounded-lg bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors flex items-center justify-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <i class="pi pi-cog text-xs text-blue-500"></i>
          <span>Nastavení aplikace</span>
        </button>
      </router-link>

      <!-- Version info -->
      <div class="mt-2 text-center text-2xs text-gray-400 dark:text-gray-500">
        v1.0 · Multi-provider LLM Chat
      </div>
    </div>
  </aside>
</template>
