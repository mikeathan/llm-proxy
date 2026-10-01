<script setup lang="ts">
import { ref, computed, nextTick, watch } from 'vue'
import type { Turn } from '../../../types/message'
import type { AssistantMessage } from '../../../types/assistant'
import { useAutoScroll } from '../../../composables/ui/useAutoScroll'
import ChatBubble from './ChatBubble.vue'
import UserMessage from './UserMessage.vue'
import BaseButton from '../../common/buttons/BaseButton.vue'
import type { InsetPhase } from '../../../types/inset'

const props = defineProps<{
  messages: AssistantMessage[]
  turns: Turn[]
  loading: boolean
  thinking: boolean
  liveReasoning: string
  paused: boolean
  workspaceId: string
  turnsCollapsed: Record<number, boolean>
  expandedSegments: Record<string, boolean>
  isInsetCollapsed: (idx: number) => boolean
  isSegExpanded: (turnIdx: number, segIdx: number) => boolean
  phase: InsetPhase
  mode?: 'chat' | 'automation'
}>()

// In automation mode there is no chat prompt, so the welcome/retry affordances
// are hidden; the synthetic run header + reasoning inset still render.
const isAutomation = computed(() => props.mode === 'automation')

const emit = defineEmits<{
  retry: [text: string]
  toggleInset: [idx: number]
  toggleSegment: [turnIdx: number, segIdx: number]
  "scroll-update": [atBottom: boolean]
}>()

const {
  container,
  scrollIfNearBottom,
  scrollToBottom,
  scrollDirection,
  toggleScroll,
  isNearBottom,
  updateWasNearBottom,
  notifyContent,
} = useAutoScroll()

const atBottom = ref(true)

function onContainerScroll() {
  updateWasNearBottom(container.value)
  atBottom.value = isNearBottom(container.value)
  emit('scroll-update', atBottom.value)
}

// Auto-scroll on new content (streaming tokens, new turns) but only while the
// user is parked at the bottom — pause when they scroll up to read, then
// resume (snap to bottom) after a short idle if fresh output arrived. Mirrors
// the automation live-run behaviour.
// Watch liveReasoning too: during a long reasoning stream the content grows
// via the liveReasoning prop while `turns` is still stable (reasoning only
// commits to a segment at a tool_call/message boundary), so without this the
// pane would overflow without ever scrolling.
// The scroll is THROTTLED (~250ms): live reasoning flushes ~10x/sec, and
// scrolling the whole pane every flush is a full-pane scroll+layout+composite
// pass (the GPU hot path during streaming). Coalescing to a few scrolls/sec
// keeps the pane following content with far less compositing. notifyContent()
// still runs every change so the pause/resume bookkeeping never misses input.
let lastScrollAt = 0
let lastScrollHeight = 0
function maybeScroll() {
  // Bookkeeping always runs so pause/resume never misses input.
  notifyContent()
  nextTick(() => {
    const now = Date.now()
    if (now - lastScrollAt < 250) return
    // Consume the throttle window BEFORE the height check: the scrollHeight
    // read is a forced synchronous layout, so it must not run on every flush
    // (10x/sec) while content is static (e.g. inset capped at 320px — the
    // outer pane stops growing and every flush would otherwise re-layout).
    lastScrollAt = now
    const el = container.value
    if (!el) return
    // Skip the scrollTop mutation entirely when nothing grew since the last
    // pass — a flush that added no content otherwise forces a pointless
    // synchronous layout + composite of the whole pane.
    if (el.scrollHeight === lastScrollHeight) return
    lastScrollHeight = el.scrollHeight
    scrollIfNearBottom(el, "instant")
    atBottom.value = isNearBottom(el)
  })
}

watch(() => props.liveReasoning, maybeScroll)
watch(() => props.thinking, maybeScroll)
// Tool/segment/message events replace the message object → turns recomputes.
// Watched by reference (not deep) — deep-walking the whole history on every
// flush is wasted work; a new turns array is enough to detect the change.
watch(() => props.turns, maybeScroll)

defineExpose({
  scrollToBottom: (behavior: ScrollBehavior = "smooth") => scrollToBottom(container.value, behavior),
})
</script>

<template>
   <div class="message-container" ref="container" @scroll="onContainerScroll">
    <slot name="run-header" :phase="phase" :is-automation="isAutomation" />

    <!-- Empty state (plan D14): says what the agent will do, and frames the
         guardrail approval as an expected step rather than an error. -->
    <div v-if="messages.length === 0 && !loading && !isAutomation" class="chat-empty">
      <p class="chat-empty-eyebrow">Assistant · <span class="font-mono normal-case">{{ workspaceId }}</span></p>
      <h3 class="chat-empty-title">Ask about this workspace</h3>
      <p>It reads and changes files in <span class="font-mono text-primary">{{ workspaceId }}</span> and uses the tools its guardrails allow.</p>
      <p>When a step is blocked, it stops and asks you — approving or denying it is part of the normal flow.</p>
      <p class="chat-empty-hint">Enter sends · Shift+Enter adds a line · Ctrl / ⌘ K focuses the message box.</p>
    </div>

    <template v-for="(turn, idx) in turns" :key="'turn-' + idx">
      <UserMessage
        v-if="!isAutomation"
        :content="turn.userMessage"
        @retry="emit('retry', turn.userMessage)"
      />

      <ChatBubble
        v-if="turn.segments.length || turn.finalAnswer || turn.canceled || (loading && idx === turns.length - 1)"
        :turn="turn"
        :idx="idx"
        :loading="loading"
        :thinking="thinking"
        :live-reasoning="liveReasoning"
        :paused="paused"
        :is-last-turn="idx === turns.length - 1"
        :phase="phase"
        :is-inset-collapsed="isInsetCollapsed(idx)"
        :is-seg-expanded="isSegExpanded"
        @toggle-inset="emit('toggleInset', idx)"
        @toggle-segment="(turnIdx, segIdx) => emit('toggleSegment', turnIdx, segIdx)"
      />
    </template>

    <BaseButton
      v-if="!atBottom"
      variant="secondary"
      size="sm"
      :icon="scrollDirection === 'down' ? 'arrow-down' : 'arrow-up'"
      icon-only
      :label="scrollDirection === 'down' ? 'Scroll to the latest message' : 'Scroll to the first message'"
      class-name="scroll-to-bottom"
      @click="toggleScroll(container)"
    />
  </div>
</template>

<style scoped>
.message-container { @apply relative flex-1 overflow-y-auto p-3 sm:p-4 md:p-6 flex flex-col gap-5; }
.chat-empty { @apply mx-auto flex max-w-[56ch] flex-col gap-2 border border-dashed border-control p-5 text-[length:var(--text-small)] text-secondary; }
.chat-empty p { @apply m-0; }
.chat-empty-eyebrow { @apply font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted; }
.chat-empty-title { @apply m-0 text-[length:var(--text-heading)] font-semibold text-primary; }
.chat-empty-hint { @apply font-mono text-[length:var(--text-micro)] text-faint; }
:deep(.scroll-to-bottom) { @apply sticky bottom-0 ml-auto z-10 bg-surface; }
</style>
