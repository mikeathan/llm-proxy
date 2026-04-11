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
</script>

<template>
  <div v-if="isOpen" class="dialog-overlay">
    <div
      class="dialog-content"
      :class="`dialog-content--${type}`"
    >
      <h3 class="dialog-title">{{ title }}</h3>
      <p class="dialog-message">{{ message }}</p>
      <div class="dialog-actions">
        <button
          @click="emit('cancel'); close()"
          class="btn-secondary"
        >
          {{ cancelText }}
        </button>
        <button
          @click="emit('confirm'); close()"
          class="btn-primary"
        >
          {{ confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.dialog-overlay {
  @apply fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm;
}

.dialog-content {
  @apply w-full max-w-md p-6 rounded-lg border shadow-xl transition-all;
}

.dialog-content--info {
  @apply bg-blue-900/20 border-blue-500 text-blue-200;
}

.dialog-content--warning {
  @apply bg-yellow-900/20 border-yellow-500 text-yellow-200;
}

.dialog-content--error {
  @apply bg-red-900/20 border-red-500 text-red-200;
}

.dialog-title {
  @apply text-lg font-semibold mb-2;
}

.dialog-message {
  @apply text-sm mb-6 opacity-90;
}

.dialog-actions {
  @apply flex justify-end gap-3;
}

.btn-secondary {
  @apply px-4 py-2 text-sm font-medium hover:bg-white/10 rounded transition-colors;
}

.btn-primary {
  @apply px-4 py-2 text-sm font-medium bg-white/10 hover:bg-white/20 rounded transition-colors;
}
</style>
