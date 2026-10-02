<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import type { LaneHolder, LaneKey, LaneKind, LaneSummary, QueuedRun } from '../../types/assistant'
import { useGlobalRunActivity } from '../../composables/assistant/useGlobalRunActivity'
import { AssistantService } from '../../services/assistant/assistantService'
import { useToast } from '../../composables/useToast'
import { useConfirm } from '../../composables/ui/useConfirm'
import { useDismissable } from '../../composables/ui/useDismissable'
import { formatElapsedSince } from '../../utils/format/time'
import { POLL_INTERVAL_MS } from '../../constants/api'
import { LANE_LABELS, RUN_KIND_LABELS } from '../../constants/runs'
import BaseButton from '../common/buttons/BaseButton.vue'
import PopoverPanel from '../common/layout/PopoverPanel.vue'
import SlotBar from '../common/display/SlotBar.vue'
import StatusTag from '../common/display/StatusTag.vue'
import MicroLabel from '../common/display/MicroLabel.vue'
import RunLabel from '../common/display/RunLabel.vue'
import { runTarget, runTitle } from '../../utils/runs/runTarget'

// The global run indicator (plan D14, Phase 5 Run pill). At rest it is a quiet
// "Idle" mark; while anything runs or waits it reads "n running · m queued".
// Open, it shows each run lane as a slot bar (one block per slot, from the
// lane limits in /active-runs) with the runs holding and waiting for it, and
// external callers waiting for a local model — the only rows the operator acts
// on. Below `sm` the panel is a bottom sheet over a scrim instead of a popover.

// Elapsed labels tick once per second, but only while the panel is open.
const TICK_MS = 1000
// The key prefix is minted by the backend (handlers.nextInboundKey) and is the
// fallback for payloads that predate the kind field.
const INBOUND_KEY_PREFIX = 'inbound:'
const INBOUND_HINT = 'An API caller waits for a model a run is using. Serving it now cancels that run.'
const UNAVAILABLE_TEXT = 'Run state unavailable'
const UNAVAILABLE_HINT =
  "Couldn't reach the run scheduler, so running and queued runs are unknown. " +
  `Retrying every ${POLL_INTERVAL_MS / 1000} seconds.`
const IDLE_LABEL = 'No runs active — show run lanes'
const FALLBACK_KIND_LABEL = 'Run'
const PILL_CLASS = {
  running: 'border-state-running/60 bg-state-running/[0.08] text-primary',
  queued: 'border-state-queued/60 text-primary',
  unavailable: 'border-state-error/60 bg-state-error/[0.08] text-state-error',
  idle: 'border-control text-muted',
} as const
const DOT_CLASS = {
  running: 'run-dot--live',
  queued: 'border border-state-queued',
  unavailable: 'bg-state-error',
  idle: 'bg-text-decorative',
} as const

const { laneHolders, queuedRuns, lanes, runningCount, queuedCount, error, lastTick, refresh } = useGlobalRunActivity()
// Renamed: the composable above already exposes `error` (the poll failure).
const { error: toastError } = useToast()
const { confirm } = useConfirm()

// A failed poll means the last lists may be stale, so they are not shown as
// live: "unavailable" is reported instead of a possibly-wrong "nothing running".
const unavailable = computed(() => error.value !== null)
const answered = computed(() => lastTick.value !== null)
const state = computed<keyof typeof PILL_CLASS>(() => {
  if (unavailable.value) return 'unavailable'
  if (runningCount.value > 0) return 'running'
  if (queuedCount.value > 0) return 'queued'
  return 'idle'
})

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
useDismissable(root, open)

const holderLabel = (holder: LaneHolder): string => runTitle(holder) || holder.workspace_id
const queuedLabel = (entry: QueuedRun): string => runTitle(entry) || entry.workspace_id
const kindLabel = (kind: LaneKind | undefined): string => (kind ? RUN_KIND_LABELS[kind] : undefined) ?? FALLBACK_KIND_LABEL
const laneLabel = (lane: LaneKey): string => LANE_LABELS[lane] ?? lane

// An inbound row is an external /v1 caller waiting for the local model — the
// only queued rows the operator can act on.
const isInbound = (entry: QueuedRun): boolean => entry.kind === 'inbound' || entry.key.startsWith(INBOUND_KEY_PREFIX)

const laneKeys = computed(() => new Set(lanes.value.flatMap((lane) => lane.holder_keys)))
const holdersOf = (lane: LaneSummary) => laneHolders.value.filter((h) => lane.holder_keys.includes(h.key))
const queuedOf = (lane: LaneSummary) => queuedRuns.value.filter((q) => !isInbound(q) && (q.lane ?? 'local') === lane.lane)
// Runs no lane claims: inbound callers being served (they hold a model, not a
// lane slot), or every run when the backend reports no lane summary.
const otherHolders = computed(() => laneHolders.value.filter((h) => !laneKeys.value.has(h.key)))
const inboundQueued = computed(() => queuedRuns.value.filter(isInbound))
const unlanedQueued = computed(() =>
  lanes.value.length ? [] : queuedRuns.value.filter((q) => !isInbound(q)),
)
const showOther = computed(() => otherHolders.value.length + inboundQueued.value.length + unlanedQueued.value.length > 0)

// One action at a time per row, so a slow promote cannot be double-fired.
const busyKey = ref<string | null>(null)

async function runQueueAction(key: string, action: (key: string) => Promise<void>, label: string) {
  if (busyKey.value) return
  busyKey.value = key
  try {
    await action(key)
    await refresh()
  } catch (e) {
    toastError(e instanceof Error ? e.message : `Could not ${label} the queued caller`)
  } finally {
    busyKey.value = null
  }
}

async function promote(entry: QueuedRun) {
  const ok = await confirm({
    title: `Serve ${queuedLabel(entry)} now?`,
    message: 'The run using the model it waits for is cancelled; the caller is served once that run stops.',
    type: 'warning',
    confirmText: 'Serve now',
  })
  if (ok) await runQueueAction(entry.key, AssistantService.promoteQueuedRun, 'serve')
}
const dismiss = (entry: QueuedRun) => runQueueAction(entry.key, AssistantService.cancelQueuedRun, 'dismiss')
</script>

<template>
  <div ref="root" class="relative flex-none">
    <button
      v-if="answered"
      type="button"
      :data-state="state"
      :aria-label="state === 'idle' ? IDLE_LABEL : undefined"
      :aria-expanded="open"
      aria-controls="run-activity-panel"
      :class="[
        'flex h-[30px] items-center gap-2 whitespace-nowrap rounded-[var(--radius-md)] border px-2.5 font-mono text-[length:var(--text-small)] transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2',
        PILL_CLASS[state],
      ]"
      @click="open = !open"
    >
      <span aria-hidden="true" :class="['h-2 w-2 flex-none rounded-full', DOT_CLASS[state]]"></span>
      <span v-if="state === 'unavailable'">{{ UNAVAILABLE_TEXT }}</span>
      <span v-else-if="state === 'idle'" class="max-sm:sr-only">Idle</span>
      <span v-else>
        <span v-if="runningCount > 0"><span class="tabular-nums">{{ runningCount }}</span> running</span>
        <span v-if="runningCount > 0 && queuedCount > 0" aria-hidden="true"> · </span>
        <span v-if="queuedCount > 0"><span class="tabular-nums">{{ queuedCount }}</span> queued</span>
      </span>
    </button>

    <PopoverPanel v-if="open && answered" id="run-activity-panel" title="Run activity" @close="open = false">
      <div v-if="unavailable" class="flex flex-col gap-2 p-3">
        <p class="m-0 text-[length:var(--text-small)] font-semibold text-state-error">{{ UNAVAILABLE_TEXT }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-secondary">{{ UNAVAILABLE_HINT }}</p>
        <p v-if="error" class="m-0 break-words font-mono text-[length:var(--text-micro)] text-muted">{{ error }}</p>
      </div>

      <template v-else>
        <section
          v-for="lane in lanes"
          :key="lane.lane"
          :data-lane="lane.lane"
          class="flex flex-col gap-2 border-b border-hairline p-3 last:border-b-0"
        >
          <SlotBar :label="`${laneLabel(lane.lane)} lane`" :used="lane.running" :limit="lane.limit" />
          <ul v-if="holdersOf(lane).length || queuedOf(lane).length" class="m-0 flex list-none flex-col gap-1.5 p-0">
            <li v-for="holder in holdersOf(lane)" :key="holder.key" class="run-row">
              <span aria-hidden="true" class="run-dot--live h-2 w-2 flex-none rounded-full"></span>
              <RunLabel :title="holderLabel(holder)" :to="runTarget(holder)" :tip="holder.model" @navigate="open = false" />
              <span class="run-tag">{{ kindLabel(holder.kind) }}</span>
              <span class="run-time">{{ formatElapsedSince(holder.since, now) }}</span>
            </li>
            <li v-for="entry in queuedOf(lane)" :key="entry.key" class="run-row">
              <StatusTag state="queued" :label="`#${entry.position}`" />
              <RunLabel :title="queuedLabel(entry)" :to="runTarget(entry)" :tip="entry.model" muted @navigate="open = false" />
              <span class="run-tag">{{ kindLabel(entry.kind) }}</span>
              <span class="run-time">{{ formatElapsedSince(entry.queued_at, now) }}</span>
            </li>
          </ul>
          <p v-else class="m-0 text-[length:var(--text-small)] text-faint">Idle</p>
        </section>

        <section v-if="showOther" data-group="external" aria-label="API callers and other runs" class="flex flex-col gap-2 p-3">
          <MicroLabel>{{ lanes.length ? 'API callers' : 'Runs' }}</MicroLabel>
          <ul class="m-0 flex list-none flex-col gap-1.5 p-0">
            <li v-for="holder in otherHolders" :key="holder.key" class="run-row">
              <span aria-hidden="true" class="run-dot--live h-2 w-2 flex-none rounded-full"></span>
              <RunLabel :title="holderLabel(holder)" :to="runTarget(holder)" :tip="holder.model" @navigate="open = false" />
              <span class="run-tag">{{ kindLabel(holder.kind) }}</span>
              <span class="run-time">{{ formatElapsedSince(holder.since, now) }}</span>
            </li>
            <li v-for="entry in [...inboundQueued, ...unlanedQueued]" :key="entry.key" class="run-row">
              <StatusTag state="queued" :label="`#${entry.position}`" />
              <RunLabel :title="queuedLabel(entry)" :to="runTarget(entry)" :tip="entry.model" muted @navigate="open = false" />
              <span class="run-tag">{{ isInbound(entry) ? kindLabel('inbound') : laneLabel(entry.lane ?? 'local') }}</span>
              <template v-if="isInbound(entry)">
                <BaseButton variant="ghost" size="sm" icon="play" icon-only :label="`Serve ${queuedLabel(entry)} now`" :disabled="busyKey !== null" @click="promote(entry)" />
                <BaseButton variant="ghost" size="sm" icon="close" icon-only :label="`Dismiss ${queuedLabel(entry)}`" :disabled="busyKey !== null" @click="dismiss(entry)" />
              </template>
              <span v-else class="run-time">{{ formatElapsedSince(entry.queued_at, now) }}</span>
            </li>
          </ul>
          <p v-if="inboundQueued.length" class="m-0 text-[length:var(--text-small)] text-muted">{{ INBOUND_HINT }}</p>
        </section>
      </template>
    </PopoverPanel>
  </div>
</template>

<style scoped lang="postcss">
.run-row {
  @apply flex min-w-0 items-center gap-2 text-[length:var(--text-small)];
}
.run-tag {
  @apply flex-none font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted;
}
.run-time {
  @apply flex-none font-mono text-[length:var(--text-micro)] tabular-nums text-faint;
}
.run-dot--live {
  background: rgb(var(--state-live));
  box-shadow: 0 0 0 3px rgb(var(--state-live) / 0.2);
}
</style>
