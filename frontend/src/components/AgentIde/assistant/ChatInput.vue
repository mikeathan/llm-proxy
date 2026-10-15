<script setup lang="ts">
import { computed, ref, toRef, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { formatDraftSize } from '../../../utils/assistant/draftSize'
import BaseButton from '../../common/buttons/BaseButton.vue'
import ArcOrbitLoader from '../../common/layout/ArcOrbitLoader.vue'
import SlashHelper from './SlashHelper.vue'
import ComposeSheet from './ComposeSheet.vue'
import { useSlashCommands } from '../../../composables/assistant/useSlashCommands'
import type { CommandInfo } from '../../../types/commands'

// The message box starts three lines tall and grows with its text up to a cap, then
// scrolls; a long draft (a big paste) says how big it is and can be expanded to read
// it before sending — in place on a roomy screen, in a full-screen editor on a
// compact one (`compact`).

const props = defineProps<{
  loading: boolean
  paused: boolean
  inputMessage: string
  /** A small screen: expanding opens the full-screen editor instead of growing the box. */
  compact?: boolean
  /** There is a saved conversation that can be reviewed for memories. */
  canReview?: boolean
}>()

const emit = defineEmits<{
  send: []
  cancel: []
  review: []
  'update:inputMessage': [value: string]
}>()

// Past either limit a draft no longer fits the compact box.
const LONG_DRAFT_LINES = 8
const LONG_DRAFT_CHARS = 600
const LINE_BREAK = '\n'
const START_ROWS = 3
// Primary actions grow to a 44px target on a touch screen.
// The toolbar buttons are text buttons: they grow in height only.
const TOOL_TOUCH = '[@media(pointer:coarse)]:h-11'
const TOUCH_TARGET = '[@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11'

const inputRef = ref<HTMLTextAreaElement | null>(null)
const expanded = ref(false)
const sheetOpen = ref(false)

// "/" opens the command menu; once a command is named it shows its usage and a live preview.
const { suggestions, active, preview, completion, withExample } = useSlashCommands(toRef(props, 'inputMessage'))

function fill(text: string) {
  emit('update:inputMessage', text)
  inputRef.value?.focus()
}
const pickCommand = (command: CommandInfo) => fill(completion(command))

// Commands start a message: the button types the "/" that opens the menu, and is offered only where one could start.
const canStartCommand = computed(() => !props.inputMessage.trim() || props.inputMessage.startsWith('/'))
function startCommand() {
  if (!props.inputMessage) fill('/')
  else inputRef.value?.focus()
}
const pickExample = (command: CommandInfo, example: string) => fill(withExample(command, example))

const lineCount = computed(() => props.inputMessage.split(LINE_BREAK).length)
const isLongDraft = computed(() => lineCount.value > LONG_DRAFT_LINES || props.inputMessage.length > LONG_DRAFT_CHARS)
const draftSize = computed(() => formatDraftSize(props.inputMessage))

const expandLabel = computed(() => (props.compact ? 'Open the full-screen editor' : expanded.value ? 'Shrink the message box' : 'Expand the message box'))
function toggleExpand() {
  if (props.compact) sheetOpen.value = true
  else expanded.value = !expanded.value
}

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
  if (!text) {
    expanded.value = false
    sheetOpen.value = false
  }
  void nextTick(resize)
})
watch(expanded, () => void nextTick(resize))

function onKeydown(e: KeyboardEvent) {
  // Tab completes the first matching command.
  const first = suggestions.value[0]
  if (e.key === 'Tab' && !e.shiftKey && first) {
    e.preventDefault()
    pickCommand(first)
    return
  }
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
// The ask panel puts the cursor in the message when it opens.
defineExpose({ focus: () => inputRef.value?.focus() })
onUnmounted(() => document.removeEventListener('keydown', onGlobalKeydown))
</script>

<template>
  <div class="input-area">
    <SlashHelper :suggestions="suggestions" :active="active" :preview="preview" @pick="pickCommand" @example="pickExample" />
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
        :rows="START_ROWS"
        :disabled="loading"
      ></textarea>
    </div>
    <!-- A long draft says how big it is on its own line, so it never crowds the controls below. -->
    <span v-if="isLongDraft" data-test="draft-size" class="self-end font-mono text-[length:var(--text-micro)] tabular-nums text-muted">{{ draftSize }}</span>
    <!-- One action row under the box, every control centred on it: the tools (start a command, review the conversation for memories) on the left; a long draft's expand, then send or stop, on the right. -->
    <div data-test="composer-actions" class="flex items-center justify-between gap-2">
      <div class="flex min-w-0 items-center gap-1">
        <BaseButton variant="ghost" size="sm" aria-label="Commands" title="Start a command, such as /remind" :disabled="!canStartCommand || loading" :class-name="TOOL_TOUCH" @click="startCommand">
          <span aria-hidden="true">/</span><span>Commands</span>
        </BaseButton>
        <BaseButton variant="ghost" size="sm" icon="memory" title="Ask the model which facts in this conversation are worth remembering" :disabled="!canReview || loading" :class-name="TOOL_TOUCH" @click="emit('review')">Memory</BaseButton>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <BaseButton
          v-if="isLongDraft"
          variant="ghost"
          size="sm"
          :icon="expanded && !compact ? 'chevron-down' : 'chevron-up'"
          icon-only
          :label="expandLabel"
          :aria-pressed="compact ? undefined : expanded"
          :class-name="TOUCH_TARGET"
          @click="toggleExpand"
        />
        <BaseButton v-if="loading" variant="danger" size="lg" icon="stop" icon-only label="Stop the run" :class-name="TOUCH_TARGET" @click="emit('cancel')" />
        <BaseButton v-else variant="primary" size="lg" icon="send" icon-only label="Send" :class-name="TOUCH_TARGET" :disabled="!inputMessage.trim()" @click="emit('send')" />
      </div>
    </div>
    <ComposeSheet
      :open="sheetOpen"
      :model-value="inputMessage"
      @update:model-value="emit('update:inputMessage', $event)"
      @send="emit('send')"
      @close="sheetOpen = false"
    />
  </div>
</template>

<style scoped>
.input-area { @apply p-3 sm:p-4 bg-canvas flex min-w-0 flex-col gap-1.5 shrink-0; }
.input-wrap { @apply relative; isolation: isolate; }
.chat-input { @apply block w-full rounded-[var(--radius-md)] border border-control bg-canvas px-3 py-2.5 text-[length:var(--text-body)] text-primary placeholder:text-faint resize-none transition-colors focus-visible:outline-none focus-visible:ring-2; position: relative; z-index: 0; max-height: min(40vh, 18rem); overflow-y: auto; }
/* The chat panel is 100vh - 10rem tall: leave room for its header and the latest messages. */
.chat-input.is-expanded { max-height: min(55vh, calc(100vh - 22rem)); }
.chat-input:disabled { @apply opacity-50 cursor-not-allowed; }
.chat-input.is-loading { @apply border-transparent; }
/* 16px on a touch screen: below that the browser zooms the page when the box is focused. */
@media (pointer: coarse) { .chat-input { font-size: 16px; } }

</style>
