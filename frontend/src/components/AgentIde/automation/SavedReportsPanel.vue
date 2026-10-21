<script setup lang="ts">
import { computed } from "vue";
import type { SavedReport } from "../../../types/dispatcher";
import type { DataTableColumn } from "../../../types/ui";
import { useSavedReports } from "../../../composables/automation/useSavedReports";
import { formatBytes } from "../../../utils/format/formatters";
import { formatAbsoluteTime, formatRelativeTime } from "../../../utils/format/time";
import Panel from "../../common/layout/Panel.vue";
import DataTable from "../../common/display/DataTable.vue";
import CopyButton from "../../common/display/CopyButton.vue";
import MarkdownViewer from "../../common/display/MarkdownViewer.vue";
import BaseButton from "../../common/buttons/BaseButton.vue";
import SavedReportLink from "./SavedReportLink.vue";

// The final reports an automation has saved (reports/<automation>/...), newest first. A row opens its report in
// place, so an older report can be read after the run history has dropped it. `latest` is the newest saved path:
// when it changes (a run just finished) the list reloads.

const props = defineProps<{
  workspaceId: string;
  automation: string;
  latest: string;
}>();

const saved = useSavedReports(
  () => props.workspaceId,
  () => props.automation,
  () => props.latest,
);

const COLUMNS: DataTableColumn<SavedReport>[] = [
  { key: "saved", label: "Saved" },
  { key: "size", label: "Size", numeric: true, value: (report) => formatBytes(report.size_bytes) },
  { key: "run", label: "Run", value: (report) => report.run_id },
];
const reportKey = (report: SavedReport) => report.path;
// With no list to show, a failure replaces the table; with a list, it is about the report that was opened.
const listError = computed(() => (saved.reports.value.length ? null : saved.error.value || null));
const openError = computed(() => (saved.reports.value.length ? saved.error.value : ""));
</script>

<template>
  <Panel title="Saved reports">
    <p v-if="openError" role="alert" class="m-0 mb-3 text-[length:var(--text-small)] text-state-error">{{ openError }}</p>
    <DataTable
      :columns="COLUMNS"
      :rows="saved.reports.value"
      :row-key="reportKey"
      caption="Saved reports of this automation"
      empty-title="No saved reports yet"
      empty-body="Each finished run keeps its report here, so you can read it after the run history has moved on."
      :loading="saved.loading.value && !saved.reports.value.length"
      :error="listError"
      activatable
      @activate="saved.open"
    >
      <template #cell-saved="{ row }">
        <time :datetime="row.started_at" :title="formatAbsoluteTime(row.started_at)" class="whitespace-nowrap font-mono text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(row.started_at) }}</time>
      </template>
    </DataTable>
    <p v-if="saved.truncated.value" class="m-0 mt-2 text-[length:var(--text-small)] text-muted">Showing the newest reports only; older ones are still in the Files page.</p>

    <article v-if="saved.selected.value" class="mt-4 flex flex-col gap-2">
      <header class="flex flex-wrap items-baseline justify-between gap-2">
        <span class="flex min-w-0 flex-wrap items-baseline gap-2 text-[length:var(--text-small)] text-muted">
          <time :datetime="saved.selected.value.started_at">{{ formatAbsoluteTime(saved.selected.value.started_at) }}</time>
          <SavedReportLink :workspace-id="workspaceId" :path="saved.selected.value.path" />
        </span>
        <span class="flex items-center gap-1">
          <CopyButton v-if="saved.content.value" :text="saved.content.value" title="Copy report" />
          <BaseButton variant="ghost" size="sm" icon="close" icon-only label="Close report" @click="saved.close" />
        </span>
      </header>
      <div v-if="saved.content.value" class="border border-hairline bg-canvas p-3">
        <MarkdownViewer :content="saved.content.value" />
      </div>
    </article>
  </Panel>
</template>
