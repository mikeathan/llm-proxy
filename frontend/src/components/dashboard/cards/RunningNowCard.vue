<script setup lang="ts">
import { computed } from "vue"
import SlotBar from "../../common/display/SlotBar.vue"
import RunRow from "../../common/display/RunRow.vue"
import EmptyState from "../../common/feedback/EmptyState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useGlobalRunActivity } from "../../../composables/assistant/useGlobalRunActivity"
import { LANE_LABELS, RUN_KIND_LABELS } from "../../../constants/runs"
import { runTarget, runTitle } from "../../../utils/runs/runTarget"
import { formatDuration } from "../../../utils/format/time"

// Dashboard card · Running now: what holds a model slot, and what waits. Small is the count; medium the list; large adds the lanes.
// The poll belongs to useGlobalRunActivity (one shared poller), so this only reads it.
const size = useCardSize()
const { laneHolders, queuedRuns, lanes, error } = useGlobalRunActivity()
const nothingRunning = computed(() => !laneHolders.value.length && !queuedRuns.value.length)
// Re-evaluated whenever the lane poll replaces the holder list, so it stays within one poll interval without a clock of its own.
const runningFor = (since: string) => formatDuration(Math.max(0, Date.now() - Date.parse(since)))
</script>

<template>
  <div data-test="running-now-card">
    <ErrorState v-if="error && nothingRunning" title="Could not read running work" :cause="error" next="It is retried automatically every few seconds." />
    <template v-else>
      <div v-if="size === 'xs'">
        <p class="m-0 font-mono text-[length:var(--text-numeral)] font-medium tabular-nums text-primary" data-test="running-count">{{ laneHolders.length }}</p>
        <p class="m-0 text-muted">running<template v-if="queuedRuns.length"> · {{ queuedRuns.length }} waiting</template></p>
      </div>
      <div v-else>
        <div v-if="size === 'lg' && lanes.length && !error" class="mb-4 grid grid-cols-[repeat(auto-fit,minmax(min(100%,160px),1fr))] gap-4">
          <SlotBar v-for="lane in lanes" :key="lane.lane" :label="`${LANE_LABELS[lane.lane] ?? lane.lane} lane`" :used="lane.running" :limit="lane.limit" />
        </div>
        <EmptyState v-if="nothingRunning" title="Nothing is running" body="Automations, chats and API callers appear here while they hold a model slot." />
        <ul v-else class="m-0 flex list-none flex-col gap-2 p-0">
          <li v-for="holder in laneHolders" :key="holder.key" class="flex min-w-0">
            <RunRow :state="holder.kind === 'inbound' ? 'info' : 'running'" :tag="RUN_KIND_LABELS[holder.kind]" :title="runTitle(holder)" :note="runningFor(holder.since)" :to="runTarget(holder)" />
          </li>
          <li v-for="entry in queuedRuns" :key="entry.key" class="flex min-w-0">
            <RunRow state="queued" :tag="`Queued #${entry.position}`" :title="runTitle(entry)" :to="runTarget(entry)" muted />
          </li>
        </ul>
      </div>
    </template>
  </div>
</template>
