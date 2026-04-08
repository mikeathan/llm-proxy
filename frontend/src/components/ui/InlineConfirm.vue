<script setup lang="ts">

defineProps<{
  message: string
  confirmText?: string
  cancelText?: string
}>()

const emit = defineEmits<{
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()
</script>

<template>
  <div class="inline-confirm">
    <div class="confirm-content">
      <span class="confirm-text">{{ message }}</span>
      <div class="confirm-actions">
        <button
          type="button"
          class="btn-cancel"
          @click.stop="emit('cancel')"
        >
          {{ cancelText || 'Cancel' }}
        </button>
        <button
          type="button"
          class="btn-confirm"
          @click.stop="emit('confirm')"
        >
          {{ confirmText || 'Yes, remove' }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.inline-confirm {
  @apply bg-red-950/20 border border-red-500/20 rounded-md p-3 my-2 mx-4 transition-all animate-in fade-in slide-in-from-top-1 duration-200;
}

.confirm-content {
  @apply flex flex-wrap items-center justify-between gap-y-2 gap-x-4;
}

.confirm-text {
  @apply text-xs text-red-300 font-medium flex-1 min-w-[120px];
}

.confirm-actions {
  @apply flex gap-2 shrink-0 ml-auto;
}

.btn-cancel {
  @apply text-gray-300 bg-gray-800 hover:bg-gray-700 text-xs px-3 py-1.5 rounded-md transition-all border border-gray-700;
}

.btn-confirm {
  @apply text-red-200 bg-red-700 hover:bg-red-600 text-xs px-3 py-1.5 rounded-md font-bold transition-all;
}
</style>
