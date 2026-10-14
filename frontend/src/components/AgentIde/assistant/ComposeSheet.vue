<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { formatDraftSize } from '../../../utils/assistant/draftSize'
import BaseButton from '../../common/buttons/BaseButton.vue'

// The full-screen editor for a long message on a small screen: the box in the chat
// is too short to read or edit a big paste with the keyboard up. It edits the same
// draft as the chat's own box, so nothing is copied back and forth. Escape and Done
// close it, focus lands in the message when it opens and returns to what opened it.
const props = defineProps<{ open: boolean; modelValue: string }>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'send'): void
  (e: 'close'): void
}>()

const field = ref<HTMLTextAreaElement | null>(null)
const size = computed(() => formatDraftSize(props.modelValue))
let opener: HTMLElement | null = null

watch(
  () => props.open,
  async (open) => {
    if (open) {
      opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
      await nextTick()
      field.value?.focus()
    } else {
      opener?.focus()
      opener = null
    }
  },
  { immediate: true },
)
</script>

<template>
  <div
    v-if="open"
    role="dialog"
    aria-modal="true"
    aria-label="Write a message"
    class="fixed inset-0 z-50 flex flex-col bg-canvas"
    @keydown.escape="emit('close')"
  >
    <header class="flex flex-none items-center gap-2 border-b border-hairline p-2">
      <BaseButton variant="ghost" size="lg" class-name="[@media(pointer:coarse)]:h-11" @click="emit('close')">Done</BaseButton>
      <span data-test="sheet-size" class="min-w-0 flex-1 truncate text-center font-mono text-[length:var(--text-micro)] tabular-nums text-muted">{{ size }}</span>
      <BaseButton variant="primary" size="lg" icon="send" class-name="[@media(pointer:coarse)]:h-11" :disabled="!modelValue.trim()" @click="emit('send')">Send</BaseButton>
    </header>
    <textarea
      ref="field"
      aria-label="Message"
      :value="modelValue"
      class="sheet-field"
      @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
    ></textarea>
  </div>
</template>

<style scoped>
/* 16px: a touch browser zooms the page when it focuses a smaller field. */
.sheet-field { @apply min-h-0 w-full flex-1 resize-none border-0 bg-transparent p-4 text-primary placeholder:text-faint focus-visible:outline-none; font-size: 16px; line-height: 1.55; }
</style>
