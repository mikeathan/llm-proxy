<script setup lang="ts">
// The user's side of a turn: the message as sent, with a retry that re-sends
// it (visible on hover and on keyboard focus). Messages carry no timestamp, so
// none is shown.
defineProps<{
  content: string
}>()

const emit = defineEmits<{
  retry: [text: string]
}>()
</script>

<template>
  <div class="group flex flex-col items-end gap-1">
    <div class="relative max-w-[85%] rounded-[var(--radius-md)] border border-hairline bg-surface-raised px-3.5 py-2.5 text-primary">
      <div class="whitespace-pre-wrap break-words text-[length:var(--text-body)] leading-relaxed">{{ content }}</div>
      <button
        type="button"
        :aria-label="`Send again: ${content}`"
        :title="`Send again: ${content}`"
        class="absolute -right-2 -top-2 flex h-6 w-6 items-center justify-center rounded-full border border-control bg-surface text-muted opacity-0 transition-opacity duration-fast hover:text-primary focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 group-hover:opacity-100"
        @click="emit('retry', content)"
      >
        <svg aria-hidden="true" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="1 4 1 10 7 10" />
          <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10" />
        </svg>
      </button>
    </div>
  </div>
</template>
