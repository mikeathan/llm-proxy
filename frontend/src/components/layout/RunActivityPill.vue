<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import type { LaneHolder, LaneKey, LaneKind, QueuedRun } from '../../types/assistant'
import { useGlobalRunActivity } from '../../composables/assistant/useGlobalRunActivity'
import { AssistantService } from '../../services/assistant/assistantService'
import { useToast } from '../../composables/useToast'
import { formatElapsedSince } from '../../utils/format/time'
import { POLL_INTERVAL_MS } from '../../constants/api'

// Elapsed labels tick once per second, but only while the panel is open.
const TICK_MS = 1000
// Copy, keys and enum labels. The key prefix is minted by the backend
// (handlers.nextInboundKey) and is the fallback for payloads that predate the
// kind field.
const INBOUND_TAG = 'External'
const INBOUND_KEY_PREFIX = 'inbound:'
const INBOUND_SERVE = 'Serve now'
const INBOUND_DISMISS = 'Dismiss'
const INBOUND_HINT = 'An external caller waits for a model a run is using. Serving it now cancels that run.'
const UNAVAILABLE_TEXT = 'Run state unavailable'
const UNAVAILABLE_HINT =
  "Couldn't reach the run scheduler, so running and queued runs are unknown. " +
  `Retrying every ${POLL_INTERVAL_MS / 1000} seconds.`
const FALLBACK_KIND_LABEL = 'Run'
// Labels for the backend runlane enums (Kind / LaneKey).
const KIND_LABELS: Record<LaneKind, string> = { automation: 'Automation', interactive: 'Chat', inbound: INBOUND_TAG }
const LANE_LABELS: Record<LaneKey, string> = { local: 'Local', cloud: 'Cloud' }

const { laneHolders, queuedRuns, runningCount, queuedCount, isActive, error, refresh } = useGlobalRunActivity()
// Renamed: the composable above already exposes `error` (the poll failure).
const { error: toastError } = useToast()

// A failed poll means the last lists may be stale, so they are not shown as
// live: "unavailable" is reported instead of a possibly-wrong "nothing running".
const unavailable = computed(() => error.value !== null)
const visible = computed(() => isActive.value || unavailable.value)

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const now = ref(Date.now())
let tick: ReturnType<typeof setInterval> | null = null

function startTick() {
  if (tick) return
  tick = setInterval(() => { now.value = Date.now() }, TICK_MS)
}

function stopTick() {
  if (tick) { clearInterval(tick); tick = null }
}

// Nothing rendered needs a clock while the panel is closed or state is unknown.
watch(() => open.value && !unavailable.value, (showElapsed) => {
  if (showElapsed) startTick()
  else stopTick()
}, { immediate: true })

onUnmounted(stopTick)

// Clicking outside the pill, or pressing Escape, dismisses the panel.
function handlePointerDown(event: PointerEvent) {
  if (!open.value) return
  if (root.value && !root.value.contains(event.target as Node)) open.value = false
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') open.value = false
}

document.addEventListener('pointerdown', handlePointerDown)
document.addEventListener('keydown', handleKeydown)
onUnmounted(() => {
  document.removeEventListener('pointerdown', handlePointerDown)
  document.removeEventListener('keydown', handleKeydown)
})

const holderLabel = (holder: LaneHolder): string => holder.label || holder.workspace_id
const queuedLabel = (entry: QueuedRun): string => entry.label || entry.workspace_id
const kindLabel = (kind: LaneKind): string => KIND_LABELS[kind] ?? FALLBACK_KIND_LABEL
const laneLabel = (lane: LaneKey): string => LANE_LABELS[lane] ?? lane

// An inbound row is an external /v1 caller waiting for the local model — the
// only queued rows the operator can act on.
const isInbound = (entry: QueuedRun): boolean => entry.kind === 'inbound' || entry.key.startsWith(INBOUND_KEY_PREFIX)
const entryTag = (entry: QueuedRun): string => (isInbound(entry) ? KIND_LABELS.inbound : laneLabel(entry.lane ?? 'local'))
const hasInbound = computed(() => queuedRuns.value.some(isInbound))

// One action at a time per row, so a slow promote cannot be double-fired.
const busyKey = ref<string | null>(null)

async function runQueueAction(key: string, action: (key: string) => Promise<void>, label: string) {
  if (busyKey.value) return
  busyKey.value = key
  try {
    await action(key)
    await refresh()
  } catch (e) {
    toastError(e instanceof Error ? e.message : `Failed to ${label} the queued caller`)
  } finally {
    busyKey.value = null
  }
}

const promote = (key: string) => runQueueAction(key, AssistantService.promoteQueuedRun, 'serve')
const dismiss = (key: string) => runQueueAction(key, AssistantService.cancelQueuedRun, 'dismiss')
</script>

<template>
  <div ref="root" class="run-activity">
    <button
      v-if="visible"
      type="button"
      class="run-pill"
      :class="
        unavailable
          ? 'run-pill--stale'
          : runningCount > 0
            ? 'run-pill--live'
            : 'run-pill--waiting'
      "
      :aria-expanded="open"
      aria-controls="run-activity-panel"
      @click="open = !open"
    >
      <span
        class="run-dot"
        :class="
          unavailable
            ? 'run-dot--stale'
            : runningCount > 0
              ? 'run-dot--live'
              : 'run-dot--queued'
        "
      ></span>
      <span v-if="unavailable">{{ UNAVAILABLE_TEXT }}</span>
      <template v-else>
        <span v-if="runningCount > 0">{{ runningCount }} running</span>
        <span v-if="queuedCount > 0">{{ queuedCount }} queued</span>
      </template>
    </button>

    <div
      v-if="open && visible"
      id="run-activity-panel"
      class="run-panel"
    >
      <div v-if="unavailable" class="panel-group">
        <h3 class="panel-heading">Run state unavailable</h3>
        <p class="panel-error">
          {{ UNAVAILABLE_HINT }}
        </p>
        <p v-if="error" class="panel-detail">{{ error }}</p>
      </div>

      <div v-if="!unavailable && runningCount > 0" class="panel-group">
        <h3 class="panel-heading">Running</h3>
        <ul class="panel-list">
          <li v-for="holder in laneHolders" :key="holder.key" class="panel-row">
            <span class="row-dot row-dot--live"></span>
            <span class="row-label" :title="holder.model">{{ holderLabel(holder) }}</span>
            <span class="row-tag">{{ kindLabel(holder.kind) }}</span>
            <span class="row-time">{{ formatElapsedSince(holder.since, now) }}</span>
          </li>
        </ul>
      </div>

      <div v-if="!unavailable && queuedCount > 0" class="panel-group">
        <h3 class="panel-heading">Queued</h3>
        <ul class="panel-list">
          <li v-for="entry in queuedRuns" :key="entry.key" class="panel-row">
            <span class="row-rank">#{{ entry.position }}</span>
            <span class="row-label" :title="entry.model">{{ queuedLabel(entry) }}</span>
            <span class="row-tag">{{ entryTag(entry) }}</span>
            <span class="row-time">{{ formatElapsedSince(entry.queued_at, now) }}</span>
            <template v-if="isInbound(entry)">
              <button
                type="button"
                class="row-action"
                :disabled="busyKey !== null"
                @click="promote(entry.key)"
              >{{ INBOUND_SERVE }}</button>
              <button
                type="button"
                class="row-action row-action--danger"
                :disabled="busyKey !== null"
                @click="dismiss(entry.key)"
              >{{ INBOUND_DISMISS }}</button>
            </template>
          </li>
        </ul>
        <p v-if="hasInbound" class="panel-hint">
          {{ INBOUND_HINT }}
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.run-activity {
  @apply relative;
}

.run-pill {
  @apply flex items-center gap-2 px-3 py-1.5 rounded-full border text-xs font-medium transition-colors;
}

.run-pill--live {
  @apply bg-green-500/10 border-green-500/30 text-green-400 hover:bg-green-500/20;
}

.run-pill--waiting {
  @apply bg-amber-500/10 border-amber-500/30 text-amber-400 hover:bg-amber-500/20;
}

.run-pill--stale {
  @apply bg-red-500/10 border-red-500/30 text-red-400 hover:bg-red-500/20;
}

.run-dot {
  @apply w-1.5 h-1.5 rounded-full shrink-0;
}

.run-dot--live {
  background: var(--color-live, #22c55e);
  box-shadow: 0 0 6px color-mix(in srgb, var(--color-live, #22c55e) 60%, transparent);
}

.run-dot--queued {
  @apply bg-amber-500;
}

.run-dot--stale {
  @apply bg-red-500;
}

.run-panel {
  @apply absolute right-0 top-full mt-2 w-72 z-20 rounded-lg border border-gray-700 bg-gray-800 p-3 shadow-xl animate-in fade-in duration-150;
}

.panel-group {
  @apply mb-3 last:mb-0;
}

.panel-heading {
  @apply text-[10px] font-bold uppercase tracking-widest text-gray-500 mb-2;
}

.panel-error {
  @apply text-xs text-red-400;
}

.panel-detail {
  @apply mt-2 text-[10px] font-mono text-gray-500 break-words;
}

.panel-list {
  @apply space-y-1.5;
}

.panel-row {
  @apply flex items-center gap-2 min-w-0 text-xs;
}

.row-dot {
  @apply w-1.5 h-1.5 rounded-full shrink-0;
}

.row-dot--live {
  background: var(--color-live, #22c55e);
}

.row-rank {
  @apply text-[10px] font-mono text-amber-400 shrink-0;
}

.row-label {
  @apply flex-1 min-w-0 truncate text-gray-200;
}

.row-tag {
  @apply text-[9px] uppercase tracking-wide text-gray-500 bg-gray-700/50 px-1.5 py-0.5 rounded shrink-0;
}

.row-time {
  @apply text-[9px] font-mono text-gray-500 shrink-0;
}

.row-action {
  @apply text-[9px] font-medium uppercase tracking-wide px-1.5 py-0.5 rounded shrink-0
    bg-blue-500/15 text-blue-300 hover:bg-blue-500/25 disabled:opacity-40 disabled:cursor-not-allowed;
}

.row-action--danger {
  @apply bg-red-500/15 text-red-300 hover:bg-red-500/25;
}

.panel-hint {
  @apply mt-2 text-[10px] leading-snug text-gray-500;
}
</style>
