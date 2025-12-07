<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Attachment } from '@/types'
import * as api from '@/api/client'
import Button from 'primevue/button'
import Textarea from 'primevue/textarea'

const props = defineProps<{
  disabled?: boolean
  isStreaming?: boolean
}>()

const emit = defineEmits<{
  (e: 'send', content: string, attachments: string[]): void
  (e: 'stop'): void
}>()

const content = ref('')
const attachments = ref<Attachment[]>([])
const isUploading = ref(false)

const canSend = computed(() => {
  return content.value.trim().length > 0 && !props.disabled && !props.isStreaming
})

function handleSend() {
  if (!canSend.value) return
  emit('send', content.value.trim(), attachments.value.map((a) => a.id))
  content.value = ''
  attachments.value = []
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    handleSend()
  }
}

async function handleNativeFileSelect(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files?.length) return

  isUploading.value = true
  try {
    for (const file of Array.from(input.files)) {
      const attachment = await api.uploadFile(file)
      attachments.value.push(attachment)
    }
  } catch (error) {
    console.error('Upload failed:', error)
  } finally {
    isUploading.value = false
    input.value = '' // Reset input for re-selection
  }
}

function removeAttachment(id: string) {
  attachments.value = attachments.value.filter((a) => a.id !== id)
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}
</script>

<template>
  <div class="border-t border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4">
    <!-- Attachments preview -->
    <div v-if="attachments.length" class="flex flex-wrap gap-2 mb-3">
      <div
        v-for="att in attachments"
        :key="att.id"
        class="relative group"
      >
        <!-- Image preview -->
        <div
          v-if="att.mime_type.startsWith('image/') && att.data"
          class="w-20 h-20 rounded-lg overflow-hidden border border-gray-200 dark:border-gray-700"
        >
          <img
            :src="`data:${att.mime_type};base64,${att.data}`"
            :alt="att.filename"
            class="w-full h-full object-cover"
          />
        </div>
        <!-- File preview -->
        <div
          v-else
          class="flex items-center gap-2 px-3 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg"
        >
          <i class="pi pi-file text-lg"></i>
          <div class="text-sm">
            <p class="truncate max-w-[6.25rem]">{{ att.filename }}</p>
            <p class="text-xs text-gray-500">{{ formatSize(att.size) }}</p>
          </div>
        </div>
        <!-- Remove button -->
        <button
          @click="removeAttachment(att.id)"
          class="absolute -top-2 -right-2 w-5 h-5 bg-red-500 text-white rounded-full flex items-center justify-center text-xs opacity-0 group-hover:opacity-100 transition-opacity"
        >
          <i class="pi pi-times"></i>
        </button>
      </div>
    </div>

    <!-- Input area -->
    <div class="flex items-end gap-2">
      <!-- File upload button -->
      <Button
        icon="pi pi-paperclip"
        @click="($refs.fileInput as HTMLInputElement)?.click()"
        :disabled="disabled || isUploading"
        :loading="isUploading"
        text
        rounded
        severity="secondary"
        v-tooltip="'Přiložit soubor'"
      />
      <input
        ref="fileInput"
        type="file"
        accept="image/*,.pdf,.txt,.md,.json,.csv"
        multiple
        class="hidden"
        @change="handleNativeFileSelect"
      />

      <!-- Text input -->
      <div class="flex-1 relative">
        <Textarea
          v-model="content"
          @keydown="handleKeydown"
          placeholder="Napište zprávu..."
          :disabled="disabled"
          rows="1"
          autoResize
          class="w-full resize-none"
          :pt="{
            root: { class: 'max-h-[12.5rem]' },
          }"
        />
      </div>

      <!-- Send / Stop button -->
      <Button
        v-if="!isStreaming"
        icon="pi pi-send"
        @click="handleSend"
        :disabled="!canSend"
        rounded
        severity="primary"
      />
      <Button
        v-else
        icon="pi pi-stop"
        @click="emit('stop')"
        severity="danger"
        rounded
      />
    </div>

    <!-- Hints -->
    <div class="mt-2 text-xs text-gray-500 flex items-center gap-4">
      <span>Enter pro odeslání, Shift+Enter pro nový řádek</span>
      <span v-if="isUploading">
        <i class="pi pi-spin pi-spinner mr-1"></i>
        Nahrávám...
      </span>
    </div>
  </div>
</template>
