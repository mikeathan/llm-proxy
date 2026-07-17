<script setup lang="ts">
import { ref, computed, nextTick, watch } from 'vue'
import type { Turn } from '../../../utils/message/turnGrouper'
import type { AssistantMessage } from '../../../types/assistant'
import { useAutoScroll } from '../../../composables/ui/useAutoScroll'
import ChatBubble from './ChatBubble.vue'
import UserMessage from './UserMessage.vue'
import Icon from '../../icons/Icon.vue'
import type { InsetPhase } from '../../../utils/message/messageBuilder.ts'

const props = defineProps<{
  messages: AssistantMessage[]
  turns: Turn[]
  loading: boolean
  thinking: boolean
  liveReasoning: string
  paused: boolean
  lastMessageIsUser: boolean
  workspaceId: string
  turnsCollapsed: Record<number, boolean>
  expandedSegments: Record<string, boolean>
  isInsetCollapsed: (idx: number) => boolean
  isSegExpanded: (turnIdx: number, segIdx: number) => boolean
  phase: InsetPhase
  error?: string | null
  mode?: 'chat' | 'automation'
}>()

// In automation mode there is no chat prompt, so the welcome/retry affordances
// are hidden; the synthetic run header + reasoning inset still render.
const isAutomation = computed(() => props.mode === 'automation')

const emit = defineEmits<{
  retry: [text: string]
  toggleInset: [idx: number]
  toggleSegment: [turnIdx: number, segIdx: number]
  dismissError: []
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

function formatStamp(): string {
  return new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
}

// Auto-scroll on new content (streaming tokens, new turns) but only while the
// user is parked at the bottom — pause when they scroll up to read, then
// resume (snap to bottom) after a short idle if fresh output arrived. Mirrors
// the automation live-run behaviour.
// Watch liveReasoning/thinking too: during a long reasoning stream the content
// grows via the liveReasoning prop while `turns` is still stable (reasoning only
// commits to a segment at a tool_call/message boundary), so without this the
// pane would overflow without ever scrolling.
watch(
  () => [props.turns, props.liveReasoning, props.thinking],
  () => {
    notifyContent()
    nextTick(() => {
      scrollIfNearBottom(container.value, "instant")
      atBottom.value = isNearBottom(container.value)
    })
  },
  { deep: true },
)

defineExpose({
  scrollToBottom: (behavior: ScrollBehavior = "smooth") => scrollToBottom(container.value, behavior),
})
</script>

<template>
   <div class="message-container" ref="container" @scroll="onContainerScroll">
    <slot name="run-header" :phase="phase" :is-automation="isAutomation" />

    <div v-if="messages.length === 0 && !loading && !isAutomation" class="chat-empty-card">
      <div class="chat-empty">
        <div class="welcome-icon"><Icon name="message" /></div>
        <h3>Workspace Assistant</h3>
        <p>You are talking to the agent bounded to <strong>{{ workspaceId }}</strong>.</p>
        <p>Ask it to scan files, check metrics, or help debug issues.</p>
      </div>
    </div>

    <div v-if="error" class="chat-error-banner">
      <span class="chat-error-text">{{ error }}</span>
      <button class="chat-error-dismiss" @click="emit('dismissError')" title="Dismiss">
        <Icon name="close" size="xs" />
      </button>
    </div>

    <template v-for="(turn, idx) in turns" :key="'turn-' + idx">
      <UserMessage
        v-if="!isAutomation"
        :content="turn.userMessage"
        :timestamp="formatStamp()"
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

    <button
      v-if="!atBottom"
      class="scroll-to-bottom"
      :title="scrollDirection === 'down' ? 'Scroll to bottom' : 'Scroll to top'"
      @click="toggleScroll(container)"
    >
      <Icon :name="scrollDirection === 'down' ? 'arrow-down' : 'arrow-up'" size="sm" />
    </button>
  </div>
</template>

<style scoped>
.message-container { @apply relative flex-1 overflow-y-auto p-3 sm:p-4 md:p-6 flex flex-col gap-5; }
.chat-empty-card { @apply bg-gray-800/40 border border-white/5 rounded-2xl p-6 mx-1; }
.chat-empty { @apply flex flex-col items-center justify-center text-center text-gray-500 gap-3 max-w-sm mx-auto; }
.welcome-icon { @apply text-4xl mb-2 opacity-50; }
.chat-error-banner { @apply flex items-start gap-2 px-3 py-2.5 mx-1 rounded-lg text-xs leading-relaxed; background: rgba(239, 68, 68, 0.1); border: 1px solid rgba(239, 68, 68, 0.25); }
.chat-error-text { @apply flex-1 text-red-300/90; }
.chat-error-dismiss { @apply shrink-0 p-0.5 rounded text-red-400/60 hover:text-red-300 hover:bg-white/5 transition-colors; }
.scroll-to-bottom { @apply absolute bottom-4 right-4 z-10 flex items-center justify-center p-2 rounded-full bg-gray-700/90 hover:bg-gray-600 text-gray-200 shadow-lg border border-white/10 transition-colors; }
</style>
