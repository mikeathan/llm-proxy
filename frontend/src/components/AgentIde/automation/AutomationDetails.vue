<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from "vue";
import type { Automation, AutomationRun } from "../../../types/dispatcher";
import type { DataTableColumn } from "../../../types/ui";
import { useLiveConsole } from "../../../composables/automation/useLiveConsole";
import { useConfirm } from "../../../composables/ui/useConfirm";
import { groupTurns } from "../../../utils/message/turnGrouper";
import { useTurnInset } from "../../../composables/ui/useTurnInset";
import { useExpandedSegments } from "../../../composables/ui/useExpandedSegments";
import { triggerLabel } from "../../../utils/automation/automationDisplay";
import { busyLabel, deliveryLabel } from "../../../utils/automation/delivery";
import { formatAbsoluteTime, formatDuration, formatRelativeTime } from "../../../utils/format/time";
import { toWorkspaceFile } from "../../../router/routes";
import Panel from "../../common/layout/Panel.vue";
import DataTable from "../../common/display/DataTable.vue";
import StatusTag from "../../common/display/StatusTag.vue";
import IdChip from "../../common/display/IdChip.vue";
import CopyButton from "../../common/display/CopyButton.vue";
import MarkdownViewer from "../../common/display/MarkdownViewer.vue";
import BaseButton from "../../common/buttons/BaseButton.vue";
import EmptyState from "../../common/feedback/EmptyState.vue";
import ChatMessages from "../assistant/ChatMessages.vue";
import GuardrailBanner from "../../common/chat/GuardrailBanner.vue";

// One automation (plan Phase 5): its configuration, the latest result, the
// live run (streamed through the same renderer as chat) and its past runs.

const props = defineProps<{
  automation: Automation;
  lastTriggerResult?: string | null;
  isExecuting?: boolean;
}>();

const emit = defineEmits<{
  (e: "open-run", run: AutomationRun): void;
  (e: "delete-run", run: AutomationRun): void;
  (e: "delete-automation-runs", automation: Automation): void;
}>();

const { confirm } = useConfirm();

const runs = computed(() =>
  [...(props.automation.history ?? [])].sort((a, b) => Date.parse(b.timestamp) - Date.parse(a.timestamp)),
);
// The run whose events replay in the console: the latest.
const activeRun = computed(() => runs.value[0] ?? null);
const showLiveUI = computed(() => !!(props.isExecuting || props.automation?.is_running));

// Unified renderer: reuse the assistant ChatMessages view for automation runs.
// useLiveConsole streams the SSE channel and feeds AgentEvents through the
// shared useMessageBuilder (same single consumer as chat), so reasoning/tool-
// calls render as proper segments instead of being overwritten into a single
// assistant message.
const {
  messages: displayMessages,
  thinking,
  liveReasoning,
  paused,
  phase,
  isConnected,
  pendingDecision,
  connect,
  disconnect,
  clearEvents,
  submitDecision,
} = useLiveConsole(
  () => props.automation.workspace,
  () => showLiveUI.value,
  () => activeRun.value?.events,
  props.automation.name,
);
const automationTurns = computed(() => groupTurns(displayMessages.value));
const { insetCollapsed, isInsetCollapsed, toggleInset } = useTurnInset(phase, automationTurns);
const { expandedSegments, isSegExpanded, toggleSegment } = useExpandedSegments();

onMounted(connect);
onUnmounted(disconnect);
watch(
  () => props.automation.workspace,
  () => {
    clearEvents();
    connect();
  },
);

// The console always seeds the run's opening message; output is anything after it.
const hasOutput = computed(() => displayMessages.value.some((m) => m.role !== "user"));
const consoleState = computed(() => {
  if (showLiveUI.value) return "Live stream";
  if (hasOutput.value) return "Audit log";
  return "Idle";
});

const config = computed(() => [
  ["Runs", triggerLabel(props.automation)],
  ["Strategy", props.automation.strategy],
  ["Model", props.automation.model || "Workspace default"],
  ["Loop strategy", props.automation.loop_strategy || "Model's setting"],
  ["Network", props.automation.network_grant || "Inherits the workspace"],
  ["Memory", props.automation.memory_mode === "hot" ? "Hot memory" : "Off"],
  ["Delivery", deliveryLabel(props.automation.notify)],
  ["When busy", busyLabel(props.automation.skip_if_busy)],
]);

async function deleteRun(run: AutomationRun) {
  const ok = await confirm({
    title: "Delete this run?",
    message: "The run and its artifacts (events, recordings, logs) are removed. This cannot be undone.",
    type: "error",
    confirmText: "Delete run",
  });
  if (ok) emit("delete-run", run);
}

async function clearRuns() {
  const ok = await confirm({
    title: `Delete every run of ${props.automation.name}?`,
    message: "Every run directory for this automation is removed. This cannot be undone.",
    type: "error",
    confirmText: "Delete all runs",
  });
  if (ok) emit("delete-automation-runs", props.automation);
}

const RUN_COLUMNS: DataTableColumn<AutomationRun>[] = [
  { key: "finished", label: "Finished" },
  { key: "status", label: "Status" },
  { key: "duration", label: "Duration", numeric: true, value: (run) => formatDuration(run.duration_ms) },
  { key: "model", label: "Model", value: (run) => run.model || "default" },
  { key: "id", label: "ID" },
  { key: "actions", label: "Actions" },
];
const runKey = (run: AutomationRun) => run.id;
</script>

<template>
  <div class="flex flex-col gap-4">
    <Panel title="Configuration">
      <dl class="m-0 grid grid-cols-[repeat(auto-fit,minmax(min(100%,220px),1fr))] gap-x-6 gap-y-3">
        <div class="flex flex-col gap-1">
          <dt class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">Task file</dt>
          <dd class="m-0">
            <RouterLink :to="toWorkspaceFile(automation.workspace, automation.task_file)" class="break-all font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline">{{ automation.task_file }}</RouterLink>
          </dd>
        </div>
        <div v-for="[term, value] in config" :key="term" class="flex flex-col gap-1">
          <dt class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">{{ term }}</dt>
          <dd class="m-0 break-words text-secondary">{{ value }}</dd>
        </div>
        <div v-if="automation.recording_ref" class="flex flex-col gap-1">
          <dt class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">Replays recording</dt>
          <dd class="m-0"><IdChip :id="automation.recording_ref" /></dd>
        </div>
      </dl>
    </Panel>

    <Panel v-if="!showLiveUI && (automation.last_error || automation.last_output)" title="Last result">
      <div v-if="automation.last_error" class="mb-3 flex flex-col gap-2">
        <span class="flex items-center justify-between gap-2">
          <StatusTag state="error" label="Failed" />
          <CopyButton :text="automation.last_error" title="Copy last error" />
        </span>
        <pre class="m-0 whitespace-pre-wrap break-words font-mono text-[length:var(--text-small)] text-state-error">{{ automation.last_error }}</pre>
      </div>
      <div v-if="automation.last_output" class="flex flex-col gap-1">
        <span class="flex justify-end">
          <CopyButton :text="automation.last_output" title="Copy last output" />
        </span>
        <MarkdownViewer :content="automation.last_output" />
      </div>
    </Panel>

    <Panel title="Console">
      <template #actions>
        <StatusTag :state="showLiveUI && isConnected ? 'running' : 'neutral'" :label="consoleState" />
      </template>
      <p v-if="showLiveUI" role="status" class="m-0 mb-3 font-mono text-[length:var(--text-small)] text-state-running">
        {{ lastTriggerResult || "Automation in progress…" }}
      </p>
      <GuardrailBanner
        v-if="pendingDecision"
        :decision="pendingDecision"
        :submit="submitDecision"
      />
      <p v-if="!showLiveUI && !hasOutput" class="m-0 text-[length:var(--text-small)] text-muted">
        Not running. A run's reasoning, tool calls and answer stream here as it happens.
      </p>
      <div v-else class="max-h-[60vh] overflow-y-auto">
        <ChatMessages
          mode="automation"
          :messages="displayMessages"
          :turns="automationTurns"
          :loading="showLiveUI"
          :thinking="thinking"
          :live-reasoning="liveReasoning"
          :paused="paused"
          :workspace-id="automation.workspace"
          :turns-collapsed="insetCollapsed"
          :expanded-segments="expandedSegments"
          :is-inset-collapsed="isInsetCollapsed"
          :is-seg-expanded="isSegExpanded"
          :phase="phase"
          @toggle-inset="toggleInset"
          @toggle-segment="toggleSegment"
        />
      </div>
    </Panel>

    <Panel title="Runs" flush>
      <template v-if="runs.length" #actions>
        <BaseButton variant="danger" size="sm" icon="trash" @click="clearRuns">Clear all runs</BaseButton>
      </template>
      <DataTable :columns="RUN_COLUMNS" :rows="runs" :row-key="runKey" caption="Runs of this automation" activatable @activate="emit('open-run', $event)">
        <template #empty>
          <EmptyState title="No runs yet" body="Run the automation, or wait for its schedule; each run appears here." />
        </template>
        <template #cell-finished="{ row }">
          <time :datetime="row.timestamp" :title="formatAbsoluteTime(row.timestamp)" class="whitespace-nowrap font-mono text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(row.timestamp) }}</time>
        </template>
        <template #cell-status="{ row }">
          <StatusTag :state="row.error ? 'error' : 'success'" :label="row.error ? 'Failed' : 'Completed'" />
        </template>
        <template #cell-id="{ row }">
          <span @click.stop><IdChip :id="row.id" /></span>
        </template>
        <template #cell-actions="{ row }">
          <span @click.stop>
            <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete run ${row.id}`" @click="deleteRun(row)" />
          </span>
        </template>
      </DataTable>
    </Panel>
  </div>
</template>
