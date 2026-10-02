<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { formatTokenCount } from '../../../utils/format/units'
import BaseButton from '../../common/buttons/BaseButton.vue'
import ArcOrbitLoader from '../../common/layout/ArcOrbitLoader.vue'

// The message box grows with its text up to a cap, then scrolls; a long draft
// (a big paste) says how big it is and can be expanded to most of the screen
// to read it before sending.

const props = defineProps<{
  loading: boolean
  paused: boolean
  inputMessage: string
}>()

const emit = defineEmits<{
  send: []
  cancel: []
  'update:inputMessage': [value: string]
}>()

// Past either limit a draft no longer fits the compact box.
const LONG_DRAFT_LINES = 8
const LONG_DRAFT_CHARS = 600
const LINE_BREAK = '\n'

const inputRef = ref<HTMLTextAreaElement | null>(null)
const expanded = ref(false)

const lineCount = computed(() => props.inputMessage.split(LINE_BREAK).length)
const isLongDraft = computed(() => lineCount.value > LONG_DRAFT_LINES || props.inputMessage.length > LONG_DRAFT_CHARS)
const draftSize = computed(() => {
  const lines = lineCount.value
  const chars = props.inputMessage.length
  return `${lines} line${lines === 1 ? '' : 's'} · ${formatTokenCount(chars)} character${chars === 1 ? '' : 's'}`
})

// Height follows the content; the CSS max-height caps it and the box scrolls.
// A hidden box (its kept-alive page in the background) reports no content
// height, so it keeps the last one instead of collapsing.
function resize() {
  const el = inputRef.value
  if (!el) return
  const previous = el.style.height
  el.style.height = 'auto'
  const content = el.scrollHeight
  el.style.height = content ? `${content}px` : previous
}

// The prop, not the input event, drives the size: a send clears it and a
// template can fill it without the user typing.
watch(() => props.inputMessage, (text) => {
  if (!text) expanded.value = false
  void nextTick(resize)
})
watch(expanded, () => void nextTick(resize))

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    emit('send')
  }
}

function onGlobalKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault()
    inputRef.value?.focus()
  }
}

onMounted(() => {
  document.addEventListener('keydown', onGlobalKeydown)
  resize()
})
onUnmounted(() => document.removeEventListener('keydown', onGlobalKeydown))
</script>

<template>
  <div class="input-area">
    <div class="flex min-w-0 flex-1 flex-col gap-1">
      <div class="input-wrap">
        <ArcOrbitLoader :active="loading && paused" :thickness="1" />
        <textarea
          ref="inputRef"
          aria-label="Message the assistant"
          :value="inputMessage"
          @input="emit('update:inputMessage', ($event.target as HTMLTextAreaElement).value)"
          @keydown="onKeydown"
          placeholder="Ask the workspace agent…"
          class="chat-input"
          :class="{ 'is-loading': loading, 'is-expanded': expanded }"
          rows="1"
          :disabled="loading"
        ></textarea>
      </div>
      <div v-if="isLongDraft" class="flex items-center justify-between gap-2">
        <span data-test="draft-size" class="font-mono text-[length:var(--text-micro)] tabular-nums text-muted">{{ draftSize }}</span>
        <BaseButton
          variant="ghost"
          size="sm"
          :icon="expanded ? 'chevron-down' : 'chevron-up'"
          icon-only
          :label="expanded ? 'Shrink the message box' : 'Expand the message box'"
          :aria-pressed="expanded"
          @click="expanded = !expanded"
        />
      </div>
    </div>
    <BaseButton v-if="loading" variant="danger" size="lg" icon="stop" icon-only label="Stop the run" @click="emit('cancel')" />
    <BaseButton v-else variant="primary" size="lg" icon="send" icon-only label="Send" :disabled="!inputMessage.trim()" @click="emit('send')" />
  </div>
</template>

<style scoped>
.input-area { @apply p-3 sm:p-4 border-t border-hairline bg-surface flex items-end gap-2 shrink-0; }
.input-wrap { @apply relative; isolation: isolate; }
.chat-input { @apply block w-full rounded-[var(--radius-md)] border border-control bg-canvas px-3 py-2.5 text-[length:var(--text-body)] text-primary placeholder:text-faint resize-none transition-colors focus-visible:outline-none focus-visible:ring-2; position: relative; z-index: 0; max-height: min(40vh, 18rem); overflow-y: auto; }
/* The chat panel is 100vh - 10rem tall: leave room for its header and the latest messages. */
.chat-input.is-expanded { max-height: min(55vh, calc(100vh - 22rem)); }
.chat-input:disabled { @apply opacity-50 cursor-not-allowed; }
.chat-input.is-loading { @apply border-transparent; }

</style>
