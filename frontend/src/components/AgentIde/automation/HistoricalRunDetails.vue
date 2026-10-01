<script setup lang="ts">
import type { AutomationRun } from "../../../types/dispatcher";
import { useConfirm } from "../../../composables/ui/useConfirm";
import { formatAbsoluteTime, formatDuration } from "../../../utils/format/time";
import MarkdownViewer from "../../common/display/MarkdownViewer.vue";
import StatusTag from "../../common/display/StatusTag.vue";
import IdChip from "../../common/display/IdChip.vue";
import MicroLabel from "../../common/display/MicroLabel.vue";
import BaseButton from "../../common/buttons/BaseButton.vue";
import ExecutionAuditTrail from "./ExecutionAuditTrail.vue";

// One finished automation run: outcome, the event trail, its error and its
// report. Shown in a drawer (Activity, Automations) or a page panel
// (Workspaces); `embedded` drops the close button when the container has one.

const props = defineProps<{
  run: AutomationRun;
  embedded?: boolean;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "delete-run", run: AutomationRun): void;
  (e: "delete-automation-runs", automation: { name: string; workspace: string }): void;
}>();

const { confirm } = useConfirm();

async function handleDeleteRun() {
  const ok = await confirm({
    title: "Delete this run?",
    message: "The run and its artifacts (events, recordings, logs) are removed. This cannot be undone.",
    type: "error",
    confirmText: "Delete run",
  });
  if (ok) emit("delete-run", props.run);
}

async function handleClearRuns() {
  if (!props.run.workspace_id) return;
  const ok = await confirm({
    title: `Delete every run of ${props.run.automation_name}?`,
    message: "Every run directory for this automation is removed. This cannot be undone.",
    type: "error",
    confirmText: "Delete all runs",
  });
  if (ok) emit("delete-automation-runs", { name: props.run.automation_name, workspace: props.run.workspace_id });
}
</script>

<template>
  <article class="flex flex-col gap-4 p-4">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div class="flex min-w-0 flex-col gap-1">
        <h2 class="m-0 text-[length:var(--text-heading)] font-semibold text-primary">{{ run.automation_name }}</h2>
        <span class="font-mono text-[length:var(--text-small)] text-muted">
          {{ run.workspace_id }} · finished {{ formatAbsoluteTime(run.timestamp) }}
        </span>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <BaseButton variant="danger" size="sm" icon="trash" @click="handleDeleteRun">Delete run</BaseButton>
        <BaseButton v-if="run.workspace_id" variant="ghost" size="sm" @click="handleClearRuns">Clear all runs</BaseButton>
        <BaseButton v-if="!embedded" variant="ghost" size="sm" icon="close" icon-only label="Close run" @click="emit('close')" />
      </div>
    </header>

    <dl class="m-0 grid grid-cols-[repeat(auto-fit,minmax(min(100%,150px),1fr))] gap-px border border-hairline bg-border-hairline">
      <div class="flex flex-col gap-1.5 bg-surface p-3">
        <dt><MicroLabel>Status</MicroLabel></dt>
        <dd class="m-0"><StatusTag :state="run.error ? 'error' : 'success'" :label="run.error ? 'Failed' : 'Completed'" /></dd>
      </div>
      <div class="flex flex-col gap-1.5 bg-surface p-3">
        <dt><MicroLabel>Duration</MicroLabel></dt>
        <dd class="m-0 font-mono tabular-nums text-primary">{{ formatDuration(run.duration_ms) }}</dd>
      </div>
      <div class="flex flex-col gap-1.5 bg-surface p-3">
        <dt><MicroLabel>Model</MicroLabel></dt>
        <dd class="m-0 font-mono text-primary">{{ run.model || "default" }}</dd>
      </div>
      <div class="flex flex-col gap-1.5 bg-surface p-3">
        <dt><MicroLabel>Run</MicroLabel></dt>
        <dd class="m-0"><IdChip :id="run.id" /></dd>
      </div>
    </dl>

    <ExecutionAuditTrail v-if="run.events?.length" :events="run.events" />

    <section v-if="run.error" class="flex flex-col gap-2">
      <MicroLabel>Final error</MicroLabel>
      <pre class="m-0 whitespace-pre-wrap break-words border border-state-error/40 bg-state-error/10 p-3 font-mono text-[length:var(--text-small)] text-state-error">{{ run.error }}</pre>
    </section>

    <section v-if="run.output" class="flex flex-col gap-2">
      <MicroLabel>Final report</MicroLabel>
      <div class="border border-hairline bg-canvas p-3">
        <MarkdownViewer :content="run.output" />
      </div>
    </section>
  </article>
</template>
