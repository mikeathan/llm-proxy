<script setup lang="ts">
import { computed, watch, nextTick } from 'vue'
import MarkdownViewer from '../../common/display/MarkdownViewer.vue'
import CopyButton from '../../common/display/CopyButton.vue'
import type { Turn } from '../../../types/message'
import Icon from '../../icons/Icon.vue'
import ToolCallSegment from './ToolCallSegment.vue'
import { useElapsedTimer } from '../../../composables/useElapsedTimer'
import { useAutoScroll } from '../../../composables/ui/useAutoScroll'
import { activityLabel, tokenUsageLabel } from '../../../constants/labels'
import { formatElapsedSeconds } from '../../../utils/format/time'
import { isFinishedPhase, isStreamingPhase, type InsetPhase } from '../../../types/inset'

// The assistant's side of a turn, read like a document: one activity line
// ("Worked 42s · 3 steps") that opens the step timeline — reasoning and tool
// calls in the order they happened — then the answer, then its actions. Only
// the live turn reserves room for the thinking indicator.

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

// Only the last turn of a running conversation is live; every earlier turn is
// finished whatever the run's current phase is.
const live = computed(() => props.loading && props.isLastTurn)
const turnPhase = computed<InsetPhase>(() => (live.value ? props.phase : 'done'))

const { seconds, start, stop } = useElapsedTimer()
watch(live, (active) => {
  if (active) start()
  else stop()
}, { immediate: true })

const toolSteps = computed(() => props.turn.segments.filter((s) => s.kind === 'tool_call').length)

// The timeline holds reasoning segments, the live-reasoning entry, and the
// generating indicator. Only show it once it actually has something to
// display — during the initial wait (thinking with no streamed text yet) it
// would otherwise paint an empty panel.
const insetHasContent = computed(() =>
  props.turn.segments.some(s => (s.kind === 'reasoning' && s.text) || s.kind === 'tool_call' || s.kind === 'guardrail' || s.kind === 'error' || s.kind === 'notice') ||
  liveReasoningVisible.value ||
  turnPhase.value === 'generating')

const insetVisible = computed(() =>
  !props.isInsetCollapsed && (insetHasContent.value || showWaitingPlaceholder.value))

// showWaitingPlaceholder surfaces a generic "waiting on the model" placeholder
// during the initial silent phase — the turn is thinking but the upstream has
// not produced any reasoning/text/segments yet (e.g. a long provider TTFT or a
// hanging connection before the first retry notice). It reuses the existing
// phase state; it is intentionally provider/model-agnostic and adds no new
// heartbeat or event. Once any content or an upstream retry notice arrives, the
// timeline shows real content and this placeholder disappears.
const showWaitingPlaceholder = computed(() =>
  live.value &&
  turnPhase.value === 'thinking' &&
  !insetHasContent.value)

const resultVisible = computed(() =>
  turnPhase.value === 'generating' || isFinishedPhase(turnPhase.value))

// Live reasoning is the in-flight thought of the CURRENT turn. It is a single
// shared ref streamed into the live turn only — historical turns render
// committed reasoning segments, never the live text, so a retry/refresh cannot
// bleed the new run's reasoning into a previous turn.
const liveReasoningVisible = computed(() =>
  props.isLastTurn && !!props.liveReasoning && isStreamingPhase(props.phase))

// The activity line shows while the turn runs, and afterwards when there is
// work to open; a turn that answered straight away has none.
const hasActivity = computed(() => live.value || props.turn.segments.length > 0)
// Measured live, or read from the turn's stored run record after a reload.
const MS_PER_SECOND = 1000
const workedSeconds = computed(() => seconds.value || Math.round((props.turn.run?.duration_ms ?? 0) / MS_PER_SECOND))
const activity = computed(() => activityLabel(turnPhase.value, toolSteps.value, workedSeconds.value))

// What the run record holds beyond the duration: the model, and the tokens
// the provider reported. Nothing is shown that was not recorded.
const runMeta = computed(() => {
  const run = props.turn.run
  if (!run || live.value) return null
  const tokens = run.completion_tokens ? tokenUsageLabel(run.prompt_tokens ?? 0, run.completion_tokens) : null
  return run.model || tokens ? { model: run.model ?? '', tokens } : null
})
const activityFailed = computed(() => props.turn.segments.some((s) => s.kind === 'error'))

// Models often open a step with "Thought:"; the timeline already labels it.
const THOUGHT_PREFIX = /^\s*thought:\s*/i
const thoughtText = (text: string) => text.replace(THOUGHT_PREFIX, '')

// A finished answer can be copied as its markdown; one still streaming cannot.
const answerCopyable = computed(() => !!props.turn.finalAnswer && !live.value)

// The timeline is height-capped (max-height + internal scroll). When live
// reasoning streams past the cap the outer container no longer grows, so the
// shared outer auto-scroll can't follow it — pin the timeline's own scroll to
// the newest line, with the same pause-on-scroll-up behaviour as the outer
// container.
const { container: insetEl, scrollIfNearBottom: scrollInsetIfNearBottom, updateWasNearBottom: onInsetScroll, notifyContent: notifyInsetContent } = useAutoScroll(50, 2000)
// Throttled (~250ms): live reasoning flushes ~10x/sec and each scroll of the
// inset is a layout+composite pass. Coalescing keeps the inset following the
// stream with far less compositing work. Gated to the last (live) turn only —
// historical turns are frozen (they can't grow the inset or be near-bottom),
// so the per-flush notifyContent/scroll bookkeeping in every historical turn
// was the audit's #6 update fan-out.
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
  <article
    :class="['turn', { 'is-live': live, 'message-wrapper--virtualized': !live }]"
    aria-label="Assistant"
  >
    <button
      v-if="hasActivity"
      type="button"
      class="activity-line"
      :aria-expanded="!isInsetCollapsed"
      @click="emit('toggleInset', idx)"
    >
      <span class="activity-chevron" :class="{ 'is-open': !isInsetCollapsed }" aria-hidden="true"><Icon name="chevron-right" size="xs" /></span>
      <span class="activity-mark" :class="{ 'is-live': live, 'is-failed': activityFailed && !live }" aria-hidden="true"></span>
      <span class="activity-label">{{ activity }}</span>
      <span v-if="live && seconds > 0" class="activity-time">{{ formatElapsedSeconds(seconds) }}</span>
    </button>

    <div class="bubble-inset-wrap" :class="{ collapsed: isInsetCollapsed }">
      <!-- v-show (not v-if): keeping the timeline mounted makes collapse/expand
           during streaming flicker-free (no re-mount + markdown re-parse on
           expand). display:none still prevents the empty-panel paint while
           hidden, so the empty-inset suppression is preserved. -->
      <div v-show="insetVisible" class="bubble-inset" ref="insetEl" @scroll="onInsetScroll(insetEl)">
        <ol class="timeline">
          <!-- Committed segments render in chronological order (reasoning and
               tool calls interleaved exactly as they streamed). -->
          <template v-for="(seg, sIdx) in turn.segments" :key="'seg-' + idx + '-' + sIdx">
            <li v-if="seg.kind === 'reasoning' && seg.text" class="timeline-item inset-reasoning">
              <span class="timeline-marker" aria-hidden="true"><Icon name="thought" size="xs" /></span>
              <div class="timeline-body">
                <span class="inset-label">Thought</span>
                <MarkdownViewer :content="thoughtText(seg.text)" variant="compact" />
              </div>
            </li>

            <ToolCallSegment
              v-else-if="seg.kind === 'tool_call'"
              :segment="seg"
              :turn-idx="idx"
              :seg-idx="sIdx"
              :expanded="isSegExpanded(idx, sIdx)"
              :compact="true"
              @toggle="(turnIdx, segIdx) => emit('toggleSegment', turnIdx, segIdx)"
            />

            <li v-else-if="seg.kind === 'guardrail'" class="timeline-item inset-guardrail">
              <span class="timeline-marker timeline-marker--error" aria-hidden="true"><Icon name="warning" size="xs" /></span>
              <div class="timeline-body">
                <span class="inset-label inset-label--error">Blocked by a guardrail</span>
                <span class="inset-guardrail-tool">{{ seg.tool }}</span>
                <span class="timeline-text">{{ seg.error }}</span>
              </div>
            </li>

            <li v-else-if="seg.kind === 'error'" class="timeline-item inset-error">
              <span class="timeline-marker timeline-marker--error" aria-hidden="true"><Icon name="close" size="xs" /></span>
              <div class="timeline-body">
                <span class="inset-label inset-label--error">Error</span>
                <span class="timeline-text">{{ seg.message }}</span>
              </div>
            </li>

            <li
              v-else-if="seg.kind === 'notice'"
              class="timeline-item inset-notice"
              :class="{ 'inset-notice--resolved': seg.status === 'resolved' }"
            >
              <span class="timeline-marker timeline-marker--running" aria-hidden="true"><Icon name="refresh" size="xs" /></span>
              <div class="timeline-body">
                <span class="inset-label inset-label--notice">Upstream</span>
                <span class="timeline-text">{{ seg.message }}</span>
              </div>
            </li>
          </template>

          <!-- Generic "waiting on the model" placeholder for the initial silent
               phase (thinking with no reasoning/content/segments yet). Rendered
               only while the last live turn is thinking with nothing to show. -->
          <li v-if="showWaitingPlaceholder" class="timeline-item inset-waiting">
            <span class="timeline-marker" aria-hidden="true"><Icon name="radio" size="xs" /></span>
            <div class="timeline-body">
              <span class="timeline-text">Waiting on the model…</span>
            </div>
          </li>

          <!-- Live (streaming) reasoning — the in-flight thought is always the
               newest event, so it renders at the tail, after committed
               segments.  v-show (not v-if) so it never unmounts, which avoids
               the panel expand/collapse flicker during fast commits. Rendered
               with full markdown via MarkdownViewer — the flush cadence
               (100–250ms, adaptive) bounds the marked() re-parse, and the GPU
               audit confirmed markdown rendering is not a GPU driver, so the
               live block keeps the same formatting as committed reasoning. -->
          <li v-show="liveReasoningVisible" class="timeline-item inset-reasoning inset-reasoning--live">
            <span class="timeline-marker" aria-hidden="true"><Icon name="thought" size="xs" /></span>
            <div class="timeline-body">
              <span class="inset-label">Thinking</span>
              <MarkdownViewer :content="liveReasoning" variant="compact" />
            </div>
          </li>

          <li v-if="turnPhase === 'generating'" class="timeline-item inset-generating">
            <span class="timeline-marker" aria-hidden="true"><Icon name="edit" size="xs" /></span>
            <div class="timeline-body">
              <span class="timeline-text">Writing the answer…</span>
            </div>
          </li>
        </ol>
      </div>
    </div>

    <div v-if="resultVisible && turn.finalAnswer" class="turn-answer">
      <MarkdownViewer :content="turn.finalAnswer" variant="document" />
    </div>

    <div v-if="answerCopyable || runMeta" class="turn-footer">
      <CopyButton v-if="answerCopyable" :text="turn.finalAnswer" title="Copy answer" />
      <span v-if="runMeta" class="turn-meta">
        <span v-if="runMeta.model">{{ runMeta.model }}</span>
        <span v-if="runMeta.model && runMeta.tokens" aria-hidden="true">·</span>
        <span v-if="runMeta.tokens" :title="runMeta.tokens.title">{{ runMeta.tokens.text }}</span>
      </span>
    </div>

    <!-- The live turn keeps this row's space (visibility, not v-if) so the
         answer does not jump as the dots come and go; finished turns drop it. -->
    <div
      v-if="live"
      class="bubble-paused"
      :class="{ 'bubble-paused--hidden': !(paused && !resultVisible) }"
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
  </article>
</template>

<style scoped>
.turn { @apply flex w-full min-w-0 flex-col gap-2 break-words; }
.message-wrapper--virtualized { content-visibility: auto; contain-intrinsic-size: auto 120px; }

/* ── Activity line ── */
.activity-line {
  @apply -ml-1.5 flex max-w-full items-center gap-2 self-start rounded-[var(--radius-md)] px-1.5 py-1 text-left font-mono text-[length:var(--text-small)] text-muted transition-colors duration-fast hover:bg-surface-hover hover:text-secondary focus-visible:outline-none focus-visible:ring-2;
  border: none; background: transparent; cursor: pointer;
}
.activity-chevron { @apply flex flex-none text-faint transition-transform duration-fast; }
.activity-chevron :deep(svg) { @apply h-3 w-3; }
.activity-chevron.is-open { transform: rotate(90deg); }
.activity-mark { @apply h-1.5 w-1.5 flex-none rounded-full bg-text-faint; }
.activity-mark.is-live { background: rgb(var(--state-live)); }
.activity-mark.is-failed { background: rgb(var(--state-error)); }
.activity-label { @apply min-w-0 truncate; }
.activity-time { @apply flex-none tabular-nums text-faint; }

/* ── Step timeline ──
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

.bubble-inset { @apply min-h-0 pl-0.5; }
/* While a run streams, cap the timeline so it stays bounded and scrolls
   internally instead of pushing the pane; a finished turn opens in full. */
.turn.is-live .bubble-inset {
  max-height: 320px;
  overflow-y: auto;
}

.timeline { @apply relative m-0 flex list-none flex-col gap-1 p-0 py-1; }
/* The rail joining the step markers. */
.timeline::before { content: ''; @apply absolute bottom-3 left-[9.5px] top-3 w-px bg-border-hairline; }
.timeline :deep(> li) { @apply relative; }

.timeline-item { @apply flex gap-2.5; }
.timeline-marker {
  @apply relative z-[1] mt-[3px] flex h-5 w-5 flex-none items-center justify-center rounded-full border border-hairline bg-canvas text-muted;
}
.timeline-marker :deep(svg) { @apply h-3 w-3; }
.timeline-marker--error { @apply border-state-error/50 text-state-error; }
.timeline-marker--running { @apply border-state-running/50 text-state-running; }
.timeline-body { @apply flex min-w-0 flex-1 flex-col gap-0.5 px-1.5 py-1; }
.timeline-text { @apply whitespace-pre-wrap break-words text-[length:var(--text-small)] leading-snug text-secondary; }

.inset-reasoning--live { @apply min-h-[1.25rem]; }
.inset-label { @apply font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted; }
.inset-label--error { @apply text-state-error; }
.inset-label--notice { @apply text-state-running; }
.inset-guardrail-tool { @apply font-mono text-[11.5px] text-state-error; }
.inset-notice--resolved { @apply opacity-60; }

/* ── Answer and its actions ── */
.turn-answer { @apply min-w-0 pt-1; }
.turn-footer { @apply -ml-1.5 flex min-w-0 items-center gap-2; }
.turn-meta { @apply flex min-w-0 items-center gap-1.5 truncate font-mono text-[length:var(--text-micro)] text-faint; }

/* ── Live thinking indicator ── */
.bubble-paused { display: flex; align-items: center; gap: 3px; padding: 2px 0; font-size: 11px; color: rgb(var(--text-muted)); }
.thinking-gap-dot { width: 5px; height: 5px; border-radius: 50%; background: rgb(var(--text-muted)); }
.bubble-paused:not(.bubble-paused--hidden) .thinking-gap-dot { animation: thinking-pulse 1.2s ease-in-out infinite; }
.bubble-paused:not(.bubble-paused--hidden) .thinking-gap-dot:nth-child(2) { animation-delay: 0.2s; }
.bubble-paused:not(.bubble-paused--hidden) .thinking-gap-dot:nth-child(3) { animation-delay: 0.4s; }
.bubble-paused-label { color: rgb(var(--text-muted)); font-size: 11px; }
.bubble-paused--hidden { visibility: hidden; }
@keyframes thinking-pulse { 0%, 60%, 100% { opacity: 0.3; } 30% { opacity: 1; } }

.bubble-canceled-banner { display: flex; align-items: center; gap: 6px; padding: 6px 8px; border-radius: var(--radius-sm); font-size: 11px; color: rgb(var(--text-muted)); background: rgb(var(--state-error) / 0.08); border: 1px solid rgb(var(--state-error) / 0.2); }
</style>
