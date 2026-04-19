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
      <div class="flex items-center gap-2 mb-2">
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" class="text-red-400"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
        <span class="confirm-text">{{ message }}</span>
      </div>
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
          {{ confirmText || 'Delete' }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.inline-confirm {
  @apply bg-red-500/[0.08] border border-red-500/30 rounded-lg p-3 my-2 shadow-2xl shadow-red-500/10 
         transition-all animate-in fade-in zoom-in-95 slide-in-from-top-2 duration-300;
}

.confirm-content {
  @apply flex flex-col;
}

.confirm-text {
  @apply text-[11px] text-red-200 font-semibold tracking-tight;
}

.confirm-actions {
  @apply flex gap-2 justify-end mt-1;
}

.btn-cancel {
  @apply text-gray-300 bg-white/10 hover:bg-white/20 text-[10px] px-3 py-1.5 rounded-md transition-all font-medium;
}

.btn-confirm {
  @apply text-white bg-red-600 hover:bg-red-500 text-[10px] px-4 py-1.5 rounded-md font-bold transition-all shadow-lg shadow-red-900/40 active:scale-95;
}
</style>
