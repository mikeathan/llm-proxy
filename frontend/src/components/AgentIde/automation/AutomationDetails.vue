<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import type { Automation, AutomationRun } from "../../../types/dispatcher";
import type { DataTableColumn } from "../../../types/ui";
import { useLiveConsole } from "../../../composables/automation/useLiveConsole";
import { useAutomationJournal } from "../../../composables/automation/useAutomationJournal";
import { useConfirm } from "../../../composables/ui/useConfirm";
import { useModels } from "../../../composables/models/useModels";
import { useProfiles } from "../../../composables/assistant/useProfiles";
import { AGENT_LABEL } from "../../../constants/labels";
import { profileLabel } from "../../../utils/assistant/profileChoice";
import { useMemoryDefaults } from "../../../composables/memory/useMemoryDefaults";
import { groupTurns } from "../../../utils/message/turnGrouper";
import { useTurnInset } from "../../../composables/ui/useTurnInset";
import { useExpandedSegments } from "../../../composables/ui/useExpandedSegments";
import { nextRunLabel, triggerLabel } from "../../../utils/automation/automationDisplay";
import { memoryModeLabel } from "../../../utils/automation/memoryMode";
import { busyLabel, deliveryLabel, journalLabel } from "../../../utils/automation/delivery";
import { formatAbsoluteTime, formatDuration, formatRelativeTime } from "../../../utils/format/time";
import { useRouter } from "vue-router";
import { toNetwork, toWorkspaceFile } from "../../../router/routes";
import SavedReportLink from "./SavedReportLink.vue";
import SavedReportsPanel from "./SavedReportsPanel.vue";
import Panel from "../../common/layout/Panel.vue";
import DataTable from "../../common/display/DataTable.vue";
import StatusTag from "../../common/display/StatusTag.vue";
import { runOutcomeTag } from "../../../utils/automation/runOutcome";
import RunWarnings from "../../common/display/RunWarnings.vue";
import IdChip from "../../common/display/IdChip.vue";
import CopyButton from "../../common/display/CopyButton.vue";
import MarkdownViewer from "../../common/display/MarkdownViewer.vue";
import BaseButton from "../../common/buttons/BaseButton.vue";
import EmptyState from "../../common/feedback/EmptyState.vue";
import Callout from "../../common/feedback/Callout.vue";
import ChatMessages from "../assistant/ChatMessages.vue";
import FindingsReport from "../../netaudit/FindingsReport.vue";
import AuditSetupCheck from "../../netaudit/AuditSetupCheck.vue";
import FailureExplanation from "../../netaudit/FailureExplanation.vue";
import NetauditToolbar from "../../netaudit/NetauditToolbar.vue";
import NetauditHelp from "../../netaudit/NetauditHelp.vue";
import { AUDITOR_PROFILE, NETWORK_POINTER_BODY, NETWORK_POINTER_TITLE } from "../../../constants/netaudit";
import { assessmentOf } from "../../../utils/netaudit/assessment";
import SegmentedControl from "../../common/forms/SegmentedControl.vue";
import ScanSummary from "../../netaudit/ScanSummary.vue";
import GuardrailBanner from "../../common/chat/GuardrailBanner.vue";

// One automation (plan Phase 5): its configuration, the latest result, the
// live run (streamed through the same renderer as chat) and its past runs.

const props = defineProps<{
  automation: Automation;
  lastTriggerResult?: string | null;
  isExecuting?: boolean;
}>()
const { automationDefaultOn } = useMemoryDefaults();;

const emit = defineEmits<{
  (e: "open-run", run: AutomationRun): void;
  (e: "delete-run", run: AutomationRun): void;
  (e: "delete-automation-runs", automation: Automation): void;
}>();

const { confirm } = useConfirm();

// The notes this automation keeps for its future runs (only fetched when it
// keeps a journal).
const journal = useAutomationJournal(
  () => props.automation.workspace,
  () => props.automation.name,
  () => !!props.automation.journal,
);

// The console holds the latest run's steps in memory only (nothing is stored per run unless
// run directories are enabled), so a finished run often has none to show.
const NO_STEPS_KEPT = "No steps to show. The console keeps the latest run's steps in memory only, so they are gone after a restart. The run's result is under Runs below.";
const NEVER_RAN = "Not running. A run's reasoning, tool calls and answer stream here as it happens.";

const runs = computed(() =>
  [...(props.automation.history ?? [])].sort((a, b) => Date.parse(b.timestamp) - Date.parse(a.timestamp)),
);
// The run whose events replay in the console: the latest.
const activeRun = computed(() => runs.value[0] ?? null);
const showLiveUI = computed(() => !!(props.isExecuting || props.automation?.is_running));

// A network audit's page is split by job, so one screen holds one thing: the results, the run as it happens, the history
// and the setup. Every other automation keeps the single page (`shown` is always true for it).
type AuditTab = "results" | "live" | "history" | "setup";
const isAudit = computed(() => props.automation.profile === AUDITOR_PROFILE);
const auditTab = ref<AuditTab>("results");
const shown = (tab: AuditTab) => !isAudit.value || auditTab.value === tab;
const auditTabs = computed(() => [
  { value: "results", label: "Results" },
  { value: "live", label: "Live run", live: showLiveUI.value },
  { value: "history", label: "History" },
  { value: "setup", label: "Setup" },
]);
// A different automation starts on its results again.
watch(() => props.automation.id, () => { auditTab.value = "results"; });
// The saved report of an audit starts with the scan data, which is the Results tab; here only the model's part is shown.
const lastResult = computed(() => (isAudit.value ? assessmentOf(props.automation.last_output ?? "") : (props.automation.last_output ?? "")));

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

// An automation with no model of its own runs on the primary model, so that is the one a run records.
const { state: modelState } = useModels();
const primaryModel = computed(() => modelState.value?.config?.primary_model ?? "");
const modelLabel = computed(() => props.automation.model || (primaryModel.value ? `Default (${primaryModel.value})` : "Workspace default"));
const runsOnModel = computed(() => props.automation.model || primaryModel.value);
const { profiles } = useProfiles();

const config = computed(() => [
  ["Runs", triggerLabel(props.automation)],
  ...(nextRunLabel(props.automation) ? [["Next run", `${nextRunLabel(props.automation)} (server time)`]] : []),
  ["Strategy", props.automation.strategy],
  ["Model", modelLabel.value],
  [AGENT_LABEL, profileLabel(profiles.value, "automation", props.automation.profile)],
  ["Loop strategy", props.automation.loop_strategy || "Model's setting"],
  ["Network", props.automation.network_grant || "Inherits the workspace"],
  ["Memory", memoryModeLabel(props.automation.memory_mode, automationDefaultOn.value)],
  ["Delivery", deliveryLabel(props.automation.notify)],
  ["When busy", busyLabel(props.automation.skip_if_busy)],
  ["Journal", journalLabel(props.automation.journal)],
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

async function clearJournal() {
  const ok = await confirm({
    title: `Clear the journal of ${props.automation.name}?`,
    message: "Its next run starts without notes from earlier runs. This cannot be undone.",
    type: "error",
    confirmText: "Clear journal",
  });
  if (ok) await journal.clear();
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
const router = useRouter();
const runKey = (run: AutomationRun) => run.id;
</script>

<template>
  <div class="flex flex-col gap-4">
    <template v-if="isAudit">
      <Callout tone="info" :title="NETWORK_POINTER_TITLE" data-test="network-pointer">
        {{ NETWORK_POINTER_BODY }}
        <template #actions><BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="router.push(toNetwork())">Open Network</BaseButton></template>
      </Callout>
      <NetauditToolbar />
      <SegmentedControl :model-value="auditTab" :options="auditTabs" label="Part of the audit" touch :tone="showLiveUI ? 'running' : undefined" @update:model-value="auditTab = $event as AuditTab" />
    </template>

    <Panel v-if="shown('setup')" title="Configuration">
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
        <div v-if="automation.scan && !isAudit" class="flex flex-col gap-1.5 sm:col-span-2" data-test="scans">
          <dt class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">Scans</dt>
          <dd class="m-0"><ScanSummary :scan="automation.scan" /></dd>
        </div>
        <div v-if="automation.recording_ref" class="flex flex-col gap-1">
          <dt class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">Replays recording</dt>
          <dd class="m-0"><IdChip :id="automation.recording_ref" /></dd>
        </div>
      </dl>
    </Panel>

    <template v-if="isAudit && shown('results')">
      <Callout v-if="automation.last_error && !showLiveUI" tone="error" title="The last run failed" data-test="last-run-failed">
        The report below is from the run before it.
        <template #actions><BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="auditTab = 'history'">See why</BaseButton></template>
      </Callout>
      <ScanSummary v-if="automation.scan" :scan="automation.scan" data-test="scans" />
      <AuditSetupCheck :profile="automation.profile" :network-grant="automation.network_grant" :automation-id="automation.id" />
      <FindingsReport :workspace-id="automation.workspace" :automation="automation.name" :busy="showLiveUI" :refresh-key="`${showLiveUI}|${activeRun?.id ?? ''}`" @watch-live="auditTab = 'live'" />
      <NetauditHelp />
    </template>

    <Panel v-if="shown('history') && !showLiveUI && (automation.last_error || automation.last_output || automation.last_warnings?.length)" title="Last result">
      <div v-if="!automation.last_error && automation.last_warnings?.length" class="mb-3 flex flex-col gap-2">
        <StatusTag v-bind="runOutcomeTag({ error: '', warnings: automation.last_warnings })" />
        <RunWarnings :warnings="automation.last_warnings" />
      </div>
      <div v-if="automation.last_error" class="mb-3 flex flex-col gap-2">
        <span class="flex items-center justify-between gap-2">
          <StatusTag state="error" label="Failed" />
          <CopyButton :text="automation.last_error" title="Copy last error" />
        </span>
        <FailureExplanation :error="automation.last_error" :automation-id="automation.id" />
      </div>
      <div v-if="lastResult" class="flex flex-col gap-1">
        <span class="flex items-baseline justify-between gap-2">
          <span v-if="automation.last_report_path" class="flex min-w-0 flex-wrap items-baseline gap-2 text-[length:var(--text-small)] text-muted">
            Saved as <SavedReportLink :workspace-id="automation.workspace" :path="automation.last_report_path" />
          </span>
          <CopyButton :text="automation.last_output" title="Copy last output" class="ml-auto" />
        </span>
        <MarkdownViewer :content="lastResult" />
      </div>
    </Panel>

    <SavedReportsPanel v-if="shown('history')" :workspace-id="automation.workspace" :automation="automation.name" :latest="automation.last_report_path ?? ''" />

    <Panel v-if="automation.journal && shown('history')" title="Journal">
      <template v-if="journal.journal.value" #actions>
        <CopyButton :text="journal.journal.value" title="Copy journal" />
        <BaseButton variant="danger" size="sm" icon="trash" :disabled="showLiveUI" @click="clearJournal">Clear journal</BaseButton>
      </template>
      <p v-if="journal.error.value" role="alert" class="m-0 mb-3 text-[length:var(--text-small)] text-state-error">{{ journal.error.value }}</p>
      <MarkdownViewer v-if="journal.journal.value" :content="journal.journal.value" />
      <p v-else-if="!journal.loading.value && !journal.error.value" class="m-0 text-[length:var(--text-small)] text-muted">
        No notes yet. The run writes its journal when it finishes, and reads it at the start of the next run.
      </p>
    </Panel>

    <Panel v-if="shown('live')" title="Console">
      <template #actions>
        <span v-if="runsOnModel" data-test="console-model" class="min-w-0 truncate font-mono text-[length:var(--text-micro)] text-faint" :title="`Runs on ${runsOnModel}`">{{ runsOnModel }}</span>
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
        {{ runs.length ? NO_STEPS_KEPT : NEVER_RAN }}
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

    <Panel v-if="shown('history')" title="Runs" flush>
      <template v-if="runs.length" #actions>
        <BaseButton variant="danger" size="sm" icon="trash" :disabled="showLiveUI" @click="clearRuns">Clear all runs</BaseButton>
      </template>
      <DataTable :columns="RUN_COLUMNS" :rows="runs" :row-key="runKey" caption="Runs of this automation" activatable @activate="emit('open-run', $event)">
        <template #empty>
          <EmptyState title="No runs yet" body="Run the automation, or wait for its schedule; each run appears here." />
        </template>
        <template #cell-finished="{ row }">
          <time :datetime="row.timestamp" :title="formatAbsoluteTime(row.timestamp)" class="whitespace-nowrap font-mono text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(row.timestamp) }}</time>
        </template>
        <template #cell-status="{ row }">
          <StatusTag v-bind="runOutcomeTag(row)" />
        </template>
        <template #cell-id="{ row }">
          <span @click.stop><IdChip :id="row.id" /></span>
        </template>
        <template #cell-actions="{ row }">
          <span @click.stop>
            <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete run ${row.id}`" :disabled="showLiveUI" @click="deleteRun(row)" />
          </span>
        </template>
      </DataTable>
    </Panel>
  </div>
</template>
