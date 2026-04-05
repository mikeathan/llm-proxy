<script setup lang="ts">
import { ref } from 'vue'

export type DialogType = 'info' | 'warning' | 'error'

interface Props {
  title: string
  message: string
  type?: DialogType
  confirmText?: string
  cancelText?: string
}

const props = withDefaults(defineProps<Props>(), {
  type: 'info',
  confirmText: 'Confirm',
  cancelText: 'Cancel'
})

const emit = defineEmits<{
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()

const isOpen = ref(false)

const open = () => { isOpen.value = true }
const close = () => { isOpen.value = false }

defineExpose({ open, close })

const typeClasses = {
  info: 'bg-blue-900/20 border-blue-500 text-blue-200',
  warning: 'bg-yellow-900/20 border-yellow-500 text-yellow-200',
  error: 'bg-red-900/20 border-red-500 text-red-200'
}
</script>

<template>
  <div v-if="isOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
    <div :class="['w-full max-w-md p-6 rounded-lg border shadow-xl', typeClasses[type]]">
      <h3 class="text-lg font-semibold mb-2">{{ title }}</h3>
      <p class="text-sm mb-6 opacity-90">{{ message }}</p>
      <div class="flex justify-end gap-3">
        <button 
          @click="emit('cancel'); close()" 
          class="px-4 py-2 text-sm font-medium hover:bg-white/10 rounded transition-colors"
        >
          {{ cancelText }}
        </button>
        <button 
          @click="emit('confirm'); close()" 
          class="px-4 py-2 text-sm font-medium bg-white/10 hover:bg-white/20 rounded transition-colors"
        >
          {{ confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>
