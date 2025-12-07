<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterView } from 'vue-router'
import Toast from 'primevue/toast'
import { useThemeStore } from '@/stores/theme'
import { useChatStore } from '@/stores/chat'

const themeStore = useThemeStore()
const chatStore = useChatStore()

onMounted(() => {
  themeStore.init()
  chatStore.loadProviders()
  chatStore.loadConversations()
})
</script>

<template>
  <Toast position="top-right" />
  <RouterView />
</template>
