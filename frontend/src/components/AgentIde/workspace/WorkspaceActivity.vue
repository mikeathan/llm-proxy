<script setup lang="ts">
import { computed } from "vue";
import type { AutomationRun } from "../../../types/dispatcher";
import { useConfirm } from "../../../composables/ui/useConfirm";
import { formatDate, formatDuration } from "../../../utils/format/time";
import MicroLabel from "../../common/display/MicroLabel.vue";
import StatusTag from "../../common/display/StatusTag.vue";
import { runOutcomeTag } from "../../../utils/automation/runOutcome";
import BaseButton from "../../common/buttons/BaseButton.vue";

// The workspace's automation run history in the Monitor drawer: newest first,
// each run opens its details; deleting one is confirmed.
const props = defineProps<{
  history: AutomationRun[];
  loading?: boolean;
}>();

const emit = defineEmits<{
  (e: "select-run", run: AutomationRun): void;
  (e: "delete-run", run: AutomationRun): void;
}>();

const { confirm } = useConfirm();

const sortedHistory = computed(() =>
  [...props.history].sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()),
);

async function deleteRun(run: AutomationRun) {
  const ok = await confirm({
    title: `Delete this run of ${run.automation_name}?`,
    message: "The run and its artifacts are removed. This cannot be undone.",
    type: "warning",
    confirmText: "Delete run",
  });
  if (ok) emit("delete-run", run);
}
</script>

<template>
  <section aria-label="Run history" class="flex h-full flex-col">
    <div class="flex items-center justify-between gap-2 border-b border-hairline px-4 py-3">
      <span class="flex items-center gap-3">
        <MicroLabel>Run history</MicroLabel>
        <span class="font-mono text-[length:var(--text-micro)] tabular-nums text-muted">{{ sortedHistory.length }} runs</span>
      </span>
      <span v-if="loading" role="status" class="font-mono text-[length:var(--text-micro)] text-faint">Updating…</span>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto p-4">
      <p v-if="sortedHistory.length === 0" class="m-0 py-6 text-center text-[length:var(--text-small)] text-faint">No runs yet</p>

      <ul v-else class="m-0 flex list-none flex-col gap-2 p-0">
        <li
          v-for="run in sortedHistory"
          :key="run.id"
          :class="[
            'flex min-w-0 items-start gap-1 rounded-[var(--radius-sm)] border border-hairline border-l-2 bg-canvas',
            run.error ? 'border-l-state-error' : 'border-l-state-success',
          ]"
        >
          <button
            type="button"
            :aria-label="`Open run ${run.automation_name}`"
            class="flex min-w-0 flex-1 flex-col gap-1 rounded-[var(--radius-sm)] p-3 text-left transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2"
            @click="emit('select-run', run)"
          >
            <span class="flex min-w-0 items-center justify-between gap-2">
              <span class="min-w-0 truncate font-mono text-[length:var(--text-small)] text-primary">
                {{ run.automation_name }}
                <span v-if="run.recording_ref" class="ml-1 font-mono text-[length:var(--text-micro)] uppercase text-state-running">rec</span>
              </span>
              <span class="flex-none font-mono text-[length:var(--text-micro)] text-faint">{{ formatDate(run.timestamp) }}</span>
            </span>
            <span class="truncate text-[length:var(--text-micro)] text-muted" :title="run.model || ''">{{ run.model || "Default" }}</span>
            <span class="flex items-center gap-2">
              <StatusTag v-bind="runOutcomeTag(run)" />
              <span class="font-mono text-[length:var(--text-micro)] tabular-nums text-faint">{{ run.id.slice(-6) }} · {{ formatDuration(run.duration_ms) }}</span>
            </span>
          </button>
          <BaseButton
            variant="ghost"
            size="sm"
            icon="trash"
            icon-only
            :label="`Delete run ${run.automation_name}`"
            class-name="mr-1 mt-2"
            @click="deleteRun(run)"
          />
        </li>
      </ul>
    </div>
  </section>
</template>
