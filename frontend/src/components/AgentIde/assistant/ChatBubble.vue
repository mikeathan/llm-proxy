<script setup lang="ts">
import { computed, watch, nextTick } from 'vue'
import MarkdownViewer from '../../common/display/MarkdownViewer.vue'
import type { Turn } from '../../../types/message'
import Icon from '../../icons/Icon.vue'
import ArcOrbitLoader from '../../common/layout/ArcOrbitLoader.vue'
import ToolCallSegment from './ToolCallSegment.vue'
import { useElapsedTimer } from '../../../composables/useElapsedTimer'
import { useAutoScroll } from '../../../composables/ui/useAutoScroll'
import { getPhaseLabel } from '../../../constants/labels'
import { isFinishedPhase, isStreamingPhase, type InsetPhase } from '../../../types/inset'

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

// The inset holds reasoning segments, the live-reasoning box, and the
// generating indicator.  Only show it once it actually has something to
// display — during the initial wait (thinking with no streamed text yet) it
// would otherwise paint as an empty rounded panel.  The header, animated
// border and "Thinking" dots already convey activity on their own.
const insetHasContent = computed(() =>
  props.turn.segments.some(s => (s.kind === 'reasoning' && s.text) || s.kind === 'tool_call' || s.kind === 'guardrail' || s.kind === 'error' || s.kind === 'notice') ||
  liveReasoningVisible.value ||
  props.phase === 'generating')

const insetVisible = computed(() =>
  !props.isInsetCollapsed && (insetHasContent.value || showWaitingPlaceholder.value))

// showWaitingPlaceholder surfaces a generic "waiting on the model" placeholder
// during the initial silent phase — the turn is thinking but the upstream has
// not produced any reasoning/text/segments yet (e.g. a long provider TTFT or a
// hanging connection before the first retry notice). It reuses the existing
// phase state; it is intentionally provider/model-agnostic and adds no new
// heartbeat or event. Once any content or an upstream retry notice arrives, the
// inset shows real content and this placeholder disappears.
const showWaitingPlaceholder = computed(() =>
  props.isLastTurn &&
  props.loading &&
  props.phase === 'thinking' &&
  !insetHasContent.value)

const resultVisible = computed(() =>
  props.phase === 'generating' || isFinishedPhase(props.phase))

// Live reasoning is the in-flight thought of the CURRENT turn. It is a single
// shared ref streamed into the live bubble only — historical bubbles render
// committed reasoning segments, never the live text, so a retry/refresh cannot
// bleed the new run's reasoning into a previous turn's bubble.
const liveReasoningVisible = computed(() =>
  props.isLastTurn && !!props.liveReasoning && isStreamingPhase(props.phase))

const phaseLabel = computed(() =>
  getPhaseLabel(props.phase, toolSegments.value.length))

// Show the elapsed timer in the header once the turn is actually running
// (thinking/working) or finished (done) — i.e. never during the idle wait or
// the short generating flash. Keeps the inline template condition to one token.
const showHeaderTime = computed(() =>
  isStreamingPhase(props.phase) || isFinishedPhase(props.phase))

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
      // Consume the throttle window BEFORE the overflow check so the
      // scrollHeight layout read runs at most ~4x/sec, not every flush.
      lastInsetScrollAt = now
      const el = insetEl.value
      // Skip entirely when the inset is not overflowing: a scrollTop write
      // against a scroll container that fits its content is a pointless
      // layout+composite pass every flush. Only the capped inset (reasoning
      // past the 320px cap) needs its own follow-the-newest-line scroll; the
      // outer pane covers the growth while the inset still fits.
      if (!el || el.scrollHeight <= el.clientHeight) return
      scrollInsetIfNearBottom(el, "instant")
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
        <span v-if="showHeaderTime && seconds > 0" class="bubble-header-time">· {{ seconds }}s</span>
      </button>

      <div class="bubble-inset-wrap" :class="{ collapsed: isInsetCollapsed }">
        <!-- v-show (not v-if): keeping the inset mounted makes collapse/expand
             during streaming flicker-free (no re-mount + markdown re-parse on
             expand). display:none still prevents the empty-panel paint while
             hidden, so the empty-inset suppression is preserved. -->
        <div v-show="insetVisible" class="bubble-inset" ref="insetEl" @scroll="onInsetScroll(insetEl)">
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

            <div
              v-else-if="seg.kind === 'notice'"
              class="inset-notice"
              :class="{ 'inset-notice--resolved': seg.status === 'resolved' }"
            >
              <span class="inset-label inset-label--notice">Upstream</span>
              <span class="inset-notice-message">{{ seg.message }}</span>
            </div>
          </template>

          <!-- Generic "waiting on the model" placeholder for the initial silent
               phase (thinking with no reasoning/content/segments yet). Rendered
               only while the last live turn is thinking with nothing to show. -->
          <div v-if="showWaitingPlaceholder" class="inset-waiting">
            <span class="inset-label">Upstream</span>
            <span class="inset-waiting-message">Waiting on the model…</span>
          </div>

          <!-- Live (streaming) reasoning — the in-flight thought is always the
               newest event, so it renders at the tail, after committed
               segments.  v-show (not v-if) so it never unmounts, which avoids
               the panel expand/collapse flicker during fast commits. Rendered
               with full markdown via MarkdownViewer — the flush cadence
               (100–250ms, adaptive) bounds the marked() re-parse, and the GPU
               audit confirmed markdown rendering is not a GPU driver, so the
               live block keeps the same formatting as committed reasoning. -->
          <div
            v-show="liveReasoningVisible"
            class="inset-reasoning inset-reasoning--live"
          >
            <span class="inset-label">Reasoning</span>
            <MarkdownViewer :content="liveReasoning" />
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
        :class="{ 'bubble-paused--hidden': !(loading && isLastTurn && paused && !resultVisible) }"
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

.message-bubble { @apply max-w-full sm:max-w-[85%] rounded-2xl p-3 sm:p-4 flex flex-col gap-2 shadow-sm relative break-words z-0; }
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
.inset-reasoning--live { @apply min-h-[1.25rem]; }
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

.inset-notice {
  @apply mb-2 pl-2 border-l-2 border-amber-500/70 bg-amber-500/10 rounded-r py-1.5 pr-2;
}
.inset-notice--resolved { @apply opacity-60 border-amber-500/40 bg-amber-500/5; }
.inset-label--notice { @apply text-amber-400/80; }
.inset-notice-message {
  @apply text-[11px] leading-snug text-amber-100/90 whitespace-pre-wrap break-words;
}

.inset-waiting {
  @apply mb-2 pl-2 border-l-2 border-indigo-500/40 bg-indigo-500/5 rounded-r py-1.5 pr-2;
}
.inset-waiting-message {
  @apply text-[11px] leading-snug text-indigo-200/80;
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
