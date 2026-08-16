<script setup lang="ts">
import { computed, watch, nextTick } from 'vue'
import MarkdownViewer from '../../common/display/MarkdownViewer.vue'
import type { Turn } from '../../../utils/message/turnGrouper'
import Icon from '../../icons/Icon.vue'
import ArcOrbitLoader from '../../common/layout/ArcOrbitLoader.vue'
import ToolCallSegment from './ToolCallSegment.vue'
import { useElapsedTimer } from '../../../composables/useElapsedTimer'
import { useAutoScroll } from '../../../composables/ui/useAutoScroll'
import type { InsetPhase } from '../../../utils/message/messageBuilder'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  turn: Turn
  idx: number
  loading: boolean
  thinking: boolean
  liveReasoning: string
  paused: boolean
  isLastTurn: boolean
  phase: InsetPhase
  isInsetCollapsed: boolean
  isSegExpanded: (turnIdx: number, segIdx: number) => boolean
}>()

const emit = defineEmits<{
  toggleInset: [idx: number]
  toggleSegment: [turnIdx: number, segIdx: number]
}>()

const { seconds, start, stop } = useElapsedTimer()
watch(() => props.loading && props.isLastTurn, (active) => {
  if (active) start()
  else stop()
}, { immediate: true })

const toolSegments = computed(() =>
  props.turn.segments.filter(s => s.kind === 'tool_call'))

// Reasoning/tool deltas are still arriving — the turn hasn't produced its
// final answer yet.  Drives both inset visibility and the live-reasoning box.
const isStreamingPhase = computed(() =>
  props.phase === 'thinking' || props.phase === 'working')

// The inset holds reasoning segments, the live-reasoning box, and the
// generating indicator.  Only show it once it actually has something to
// display — during the initial wait (thinking with no streamed text yet) it
// would otherwise paint as an empty rounded panel.  The header, animated
// border and "Thinking" dots already convey activity on their own.
const insetHasContent = computed(() =>
  props.turn.segments.some(s => (s.kind === 'reasoning' && s.text) || s.kind === 'tool_call' || s.kind === 'guardrail' || s.kind === 'error') ||
  liveReasoningVisible.value ||
  props.phase === 'generating')

const insetVisible = computed(() =>
  !props.isInsetCollapsed && insetHasContent.value)

const resultVisible = computed(() =>
  props.phase === 'generating' || props.phase === 'done')

const liveReasoningVisible = computed(() =>
  !!props.liveReasoning && isStreamingPhase.value)

const phaseLabel = computed(() => {
  switch (props.phase) {
    case 'thinking':   return 'Assistant — thinking...'
    case 'working':    return `Assistant — working (${toolSegments.value.length} tools)`
    case 'generating': return 'Assistant — generating answer...'
    case 'done':       return `Assistant — ${toolSegments.value.length} tools · completed`
    default:           return 'Assistant'
  }
})

// The inset is height-capped (max-height + internal scroll). When live reasoning
// streams past the cap the outer container no longer grows, so the shared outer
// auto-scroll can't follow it — pin the inset's own scroll to the newest line,
// with the same pause-on-scroll-up behaviour as the outer container.
const { container: insetEl, scrollIfNearBottom: scrollInsetIfNearBottom, updateWasNearBottom: onInsetScroll, notifyContent: notifyInsetContent } = useAutoScroll(50, 2000)
// Throttled (~250ms): live reasoning flushes ~10x/sec and each scroll of the
// inset is a layout+composite pass. Coalescing keeps the inset following the
// stream with far less compositing work. Gated to the last (live) bubble only —
// historical bubbles have frozen turns (frozen turns can't grow the inset or
// be near-bottom), so the per-flush notifyContent/scroll bookkeeping in every
// historical bubble was the audit's #6 update fan-out.
let lastInsetScrollAt = 0
watch(
  () => props.liveReasoning,
  () => {
    if (!props.isLastTurn) return
    notifyInsetContent()
    nextTick(() => {
      const now = Date.now()
      if (now - lastInsetScrollAt < 250) return
      lastInsetScrollAt = now
      scrollInsetIfNearBottom(insetEl.value, "instant")
    })
  },
)
</script>

<template>
  <div
    :class="['message-wrapper', 'message-wrapper--assistant', { 'is-loading': loading && isLastTurn, 'message-wrapper--virtualized': !(loading && isLastTurn) }]"
  >
    <div class="message-bubble message-bubble--assistant">
      <ArcOrbitLoader v-if="!turn.canceled" :active="loading && isLastTurn && paused" radius="1rem" />

      <button
        v-if="!turn.canceled || turn.segments.length > 0"
        class="bubble-header"
        @click="emit('toggleInset', idx)"
        :class="{ 'bubble-header--clickable': turn.segments.length > 0 }"
      >
        <span class="bubble-chevron" :class="{ collapsed: isInsetCollapsed }">▶</span>
        <span class="bubble-phase">{{ phaseLabel }}</span>
        <span v-if="phase === 'done' && seconds > 0" class="bubble-header-time">· {{ seconds }}s</span>
      </button>

      <div class="bubble-inset-wrap" :class="{ collapsed: isInsetCollapsed }">
        <div v-if="insetVisible" class="bubble-inset" ref="insetEl" @scroll="onInsetScroll(insetEl)">
          <!-- Committed segments render in chronological order (reasoning and
               tool calls interleaved exactly as they streamed). -->
          <template v-for="(seg, sIdx) in turn.segments" :key="'seg-' + idx + '-' + sIdx">
            <div v-if="seg.kind === 'reasoning' && seg.text" class="inset-reasoning">
              <span class="inset-label">Reasoning</span>
              <MarkdownViewer :content="seg.text" />
            </div>

            <ToolCallSegment
              v-else-if="seg.kind === 'tool_call'"
              :segment="seg"
              :turn-idx="idx"
              :seg-idx="sIdx"
              :expanded="isSegExpanded(idx, sIdx)"
              :compact="true"
              @toggle="(turnIdx, segIdx) => emit('toggleSegment', turnIdx, segIdx)"
            />

            <div
              v-else-if="seg.kind === 'guardrail'"
              class="inset-guardrail"
            >
              <span class="inset-label inset-label--guardrail">Guardrail Blocked</span>
              <div class="inset-guardrail-body">
                <span class="inset-guardrail-tool">{{ seg.tool }}</span>
                <span class="inset-guardrail-error">{{ seg.error }}</span>
              </div>
            </div>

            <div
              v-else-if="seg.kind === 'error'"
              class="inset-error"
            >
              <span class="inset-label inset-label--error">Error</span>
              <span class="inset-error-message">{{ seg.message }}</span>
            </div>
          </template>

          <!-- Live (streaming) reasoning — the in-flight thought is always the
               newest event, so it renders at the tail, after committed
               segments.  v-show (not v-if) so it never unmounts, which avoids
               the panel expand/collapse flicker during fast commits. Rendered
               as plain text while streaming (no MarkdownViewer) to avoid the
               O(n^2) marked() re-parse on every flush; the committed reasoning
               segment re-renders with full markdown via MarkdownViewer at
               ChatBubble below. -->
          <div
            v-show="liveReasoningVisible"
            class="inset-reasoning inset-reasoning--live"
          >
            <span class="inset-label">Reasoning</span>
            <div class="inset-reasoning-text">{{ liveReasoning }}</div>
          </div>

          <div v-if="phase === 'generating'" class="inset-generating">
            <span class="pulse-dot" />
            <span class="pulse-dot" />
            <span class="pulse-dot" />
            <span class="inset-generating-label">Generating answer...</span>
          </div>
        </div>
      </div>

      <div v-if="resultVisible" class="bubble-result">
        <MarkdownViewer :content="turn.finalAnswer" />
      </div>

      <div
        class="bubble-paused"
        :class="{ 'bubble-paused--hidden': !(loading && isLastTurn && paused && phase !== 'generating' && phase !== 'done') }"
      >
        <span class="thinking-gap-dot"></span>
        <span class="thinking-gap-dot"></span>
        <span class="thinking-gap-dot"></span>
        <span class="bubble-paused-label">&nbsp;Thinking</span>
      </div>

      <div v-if="turn.canceled" class="bubble-canceled-banner">
        <Icon name="close" size="xs" />
        <span>Response interrupted — send a new message to continue</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.message-wrapper { @apply flex w-full; }
.message-wrapper--assistant { @apply justify-start; }
.message-wrapper--virtualized { content-visibility: auto; contain-intrinsic-size: auto 120px; }

.message-bubble { @apply max-w-full sm:max-w-[85%] rounded-2xl p-3 sm:p-4 flex flex-col gap-2 shadow-sm relative break-words; isolation: isolate; }
.message-bubble--assistant { @apply bg-gray-800/40 border border-white/5 text-gray-200; min-height: 60px; }
.message-bubble--assistant > :not(.arc-orbit-loader) { position: relative; z-index: 0; }

.bubble-header { @apply flex items-center gap-2 select-none w-full; background: none; border: none; padding: 0; text-align: left; font-family: inherit; color: inherit; font-size: inherit; cursor: pointer; }
.bubble-header:hover .bubble-chevron { color: #d1d5db; }
.bubble-chevron { @apply inline-block mr-1.5 text-[10px] text-gray-500 transition-transform duration-200; }
.bubble-chevron:not(.collapsed) { transform: rotate(90deg); }
.bubble-phase { @apply text-[10px] font-bold uppercase tracking-wider text-gray-400; }
.bubble-header-time { @apply text-[10px] text-gray-500 font-normal; }

/* ── Inset panel ──
   Single-child grid-collapse wrapper: animates to auto height smoothly
   (unlike the old max-height:0↔auto snap which caused a delayed layout jump).
   Exactly one child (.bubble-inset) so the grid track collapse works. */
.bubble-inset-wrap {
  display: grid;
  grid-template-rows: 1fr;
  transition: grid-template-rows 0.22s ease-out;
}
.bubble-inset-wrap.collapsed {
  grid-template-rows: 0fr;
}

.bubble-inset {
  @apply ml-2 mr-2 my-1;
  @apply border-l-2 border-indigo-500/60;
  @apply bg-gray-800/50 rounded-lg;
  @apply px-3 py-2;
  /* Cap height so a long reasoning run stays bounded inside the bubble and
     scrolls internally instead of overflowing the whole pane. */
  max-height: 320px;
  overflow-y: auto;
  min-height: 0;
}

.inset-reasoning { @apply mb-2 pl-2; }
.inset-reasoning :deep(.bubble-reasoning) { @apply text-[11px] leading-snug text-gray-400 px-0; }
.inset-reasoning--live { @apply min-h-[1.25rem]; }
.inset-reasoning-text { @apply text-[11px] leading-snug text-gray-400 whitespace-pre-wrap break-words; }
.inset-label { @apply text-[10px] uppercase tracking-wider text-indigo-400/60 mb-0.5 block; }
.inset-label--guardrail { @apply text-red-400/80; }

.inset-guardrail {
  @apply mb-2 pl-2 border-l-2 border-red-500/60 bg-red-500/5 rounded-r py-1.5 pr-2;
}
.inset-guardrail-body { @apply flex flex-col gap-0.5; }
.inset-guardrail-tool {
  @apply text-[11px] font-mono text-red-300/90;
}
.inset-guardrail-error {
  @apply text-[11px] leading-snug text-red-200/80;
}

.inset-error {
  @apply mb-2 pl-2 border-l-2 border-red-500/70 bg-red-500/10 rounded-r py-1.5 pr-2;
}
.inset-label--error { @apply text-red-400/80; }
.inset-error-message {
  @apply text-[11px] leading-snug text-red-200/90 whitespace-pre-wrap break-words;
}

.inset-generating {
  @apply flex items-center justify-center gap-1.5 py-2 mt-1;
  @apply text-indigo-400 text-xs border-t border-indigo-500/20;
}
.pulse-dot {
  @apply w-1.5 h-1.5 rounded-full bg-indigo-400;
  animation: pulse-dot 1.2s ease-in-out infinite;
}
.pulse-dot:nth-child(2) { animation-delay: 0.15s; }
.pulse-dot:nth-child(3) { animation-delay: 0.3s; }
@keyframes pulse-dot {
  0%, 80%, 100% { opacity: 0.15; transform: scale(0.8); }
  40% { opacity: 1; transform: scale(1); }
}

.bubble-result { @apply px-1 py-2 text-sm leading-relaxed; }

.bubble-paused { display: flex; align-items: center; gap: 3px; padding: 2px 6px; font-size: 11px; color: rgb(107, 114, 128); }
.thinking-gap-dot { width: 5px; height: 5px; border-radius: 50%; background: rgb(107, 114, 128); }
.bubble-paused:not(.bubble-paused--hidden) .thinking-gap-dot { animation: thinking-pulse 1.2s ease-in-out infinite; }
.bubble-paused:not(.bubble-paused--hidden) .thinking-gap-dot:nth-child(2) { animation-delay: 0.2s; }
.bubble-paused:not(.bubble-paused--hidden) .thinking-gap-dot:nth-child(3) { animation-delay: 0.4s; }
.bubble-paused-label { color: rgb(107, 114, 128); font-size: 11px; }
.bubble-paused--hidden { visibility: hidden; }
@keyframes thinking-pulse { 0%, 60%, 100% { opacity: 0.3; } 30% { opacity: 1; } }

.bubble-canceled-banner { display: flex; align-items: center; gap: 6px; padding: 6px 8px; margin: 4px 0 0; border-radius: 6px; font-size: 11px; color: #94a3b8; background: rgba(239, 68, 68, 0.08); border: 1px solid rgba(239, 68, 68, 0.2); }
</style>
