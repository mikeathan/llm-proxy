<script setup lang="ts">
import BaseButton from '../../common/buttons/BaseButton.vue'
import CopyButton from '../../common/display/CopyButton.vue'

// The user's side of a turn: the message as sent, with a ghost action row
// beneath it — send again and copy — visible on hover and on keyboard focus. The
// row floats below the message instead of reserving a line of its own, so the
// turn is not taller by a row that is mostly invisible (and never shown at all on
// a touch screen). Messages carry no timestamp, so none is shown.
defineProps<{
  content: string
}>()

const emit = defineEmits<{
  retry: [text: string]
}>()
</script>

<template>
  <div class="group relative flex flex-col items-end">
    <div class="relative max-w-[85%] rounded-[var(--radius-md)] border border-hairline bg-surface-raised px-3.5 py-2.5 text-primary">
      <div class="whitespace-pre-wrap break-words text-[length:var(--text-body)] leading-relaxed">{{ content }}</div>
    </div>
    <div class="absolute right-0 top-full z-[1] flex items-center gap-0.5 opacity-0 transition-opacity duration-fast focus-within:opacity-100 group-hover:opacity-100">
      <BaseButton
        variant="ghost"
        size="sm"
        icon="refresh"
        icon-only
        :label="`Send again: ${content}`"
        @click="emit('retry', content)"
      />
      <CopyButton :text="content" title="Copy message" />
    </div>
  </div>
</template>
