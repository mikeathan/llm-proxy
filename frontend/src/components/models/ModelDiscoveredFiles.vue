<script setup lang="ts">
import { computed } from "vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import Panel from "../common/layout/Panel.vue";
import DataTable from "../common/display/DataTable.vue";
import { formatBytes, formatParameters } from "../../utils/format/formatters";
import { formatTokenCount } from "../../utils/format/units";
import type { AvailableModel, Model } from "../../types/model";
import type { DataTableColumn } from "../../types/ui";

// Models › Engine: the GGUF files in the model directory that are not added yet, each with Add (the side panel opens on it).
// Files already added are counted, not listed again.

const props = defineProps<{ files: AvailableModel[]; models: Model[] }>();
const emit = defineEmits<{ (e: "add", file: AvailableModel): void; (e: "rescan"): void }>();

const NO_VALUE = "—";
const added = computed(() => new Set(props.models.filter((m) => m.provider === "local").map((m) => m.filename).filter(Boolean)));
const notAdded = computed(() => props.files.filter((f) => !added.value.has(f.filename)));
const addedCount = computed(() => props.files.length - notAdded.value.length);
const nameOf = (f: AvailableModel) => f.metadata?.name || f.name;

const fileKey = (f: AvailableModel) => f.filename;
const COLUMNS: DataTableColumn<AvailableModel>[] = [
  { key: "name", label: "Model" },
  { key: "file", label: "File" },
  { key: "size", label: "Size", numeric: true, value: (f) => formatBytes(f.size_bytes) },
  { key: "params", label: "Params", numeric: true, value: (f) => (f.metadata?.parameters ? formatParameters(f.metadata.parameters) : NO_VALUE) },
  { key: "context", label: "Context", numeric: true, value: (f) => (f.metadata?.context_length ? formatTokenCount(f.metadata.context_length) : NO_VALUE) },
  { key: "actions", label: "Actions" },
];
</script>

<template>
  <Panel title="Found in the model directory" flush>
    <template #actions>
      <BaseButton variant="ghost" size="sm" icon="refresh" @click="emit('rescan')">Rescan</BaseButton>
    </template>
    <DataTable
      :columns="COLUMNS"
      :rows="notAdded"
      :row-key="fileKey"
      caption="GGUF files not added yet"
      empty-title="Every file is added"
      :empty-body="files.length ? 'Each .gguf file in the model directory is a model already.' : 'No .gguf file was found in the model directory.'"
    >
      <template #cell-name="{ row }">
        <span class="text-primary">{{ nameOf(row) }}</span>
        <span v-if="row.metadata?.architecture || row.metadata?.quantization" class="ml-2 font-mono text-[length:var(--text-micro)] uppercase text-faint">
          {{ [row.metadata?.architecture, row.metadata?.quantization].filter(Boolean).join(" · ") }}
        </span>
      </template>
      <template #cell-file="{ row }">
        <span class="block max-w-[220px] truncate font-mono text-[length:var(--text-small)] text-muted" :title="row.filename">{{ row.filename }}</span>
      </template>
      <template #cell-actions="{ row }">
        <BaseButton variant="ghost" size="sm" icon="plus" @click="emit('add', row)">Add<span class="sr-only">{{ " " + nameOf(row) }}</span></BaseButton>
      </template>
    </DataTable>
    <p v-if="addedCount && notAdded.length" class="m-0 px-4 py-3 text-[length:var(--text-small)] text-muted" data-test="added-count">
      {{ addedCount }} more {{ addedCount === 1 ? "file is" : "files are" }} added already.
    </p>
  </Panel>
</template>
