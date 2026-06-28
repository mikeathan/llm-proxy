<script setup lang="ts">
defineProps<{
  message: string
}>()

const emit = defineEmits<{
  (e: "dismiss"): void
}>()
</script>

<template>
  <div class="error-banner">
    <div class="error-content">
      <div class="error-message-row">
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="h-4 w-4 shrink-0 mt-0.5"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
          />
        </svg>
        <span class="error-text">{{ message }}</span>
      </div>
      <button @click="emit('dismiss')" class="btn-dismiss" title="Dismiss error">
        <Icon name="close" size="sm" />
      </button>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.error-banner {
  @apply absolute top-0 left-0 right-0 z-50 p-3 bg-red-900/90 backdrop-blur-sm 
         border-b border-red-800/50 flex flex-col gap-2 animate-in slide-in-from-top duration-300;
}

.error-content {
  @apply flex items-start justify-between gap-3;
}

.error-message-row {
  @apply flex gap-2 text-red-200;
}

.error-text {
  @apply text-[11px] leading-tight font-medium;
}

.btn-dismiss {
  @apply shrink-0 p-1 -m-1 text-red-400 hover:text-red-100 transition-colors rounded-full hover:bg-white/10;
}
</style>
