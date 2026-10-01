<script setup lang="ts">
import { computed } from "vue";
import type { McpServer, NewMcpServerForm } from "../../types/mcp";
import type { DataTableColumn } from "../../types/ui";
import Panel from "../common/layout/Panel.vue";
import DataTable from "../common/display/DataTable.vue";
import StatusTag from "../common/display/StatusTag.vue";
import FormField from "../common/forms/FormField.vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import BaseToggle from "../common/buttons/BaseToggle.vue";

// Settings · MCP servers: each change applies immediately through the MCP API
// (no Save); useMcpServers confirms a removal.
const props = defineProps<{
  mcpServers: McpServer[];
  newMcpServer: NewMcpServerForm;
}>();

const emit = defineEmits<{
  (e: "update:newMcpServer", server: NewMcpServerForm): void;
  (e: "add"): void;
  (e: "toggle", server: McpServer): void;
  (e: "remove", name: string): void;
}>();

const localName = computed({
  get: () => props.newMcpServer.name,
  set: (name) => emit("update:newMcpServer", { ...props.newMcpServer, name }),
});
const localUrl = computed({
  get: () => props.newMcpServer.url,
  set: (url) => emit("update:newMcpServer", { ...props.newMcpServer, url }),
});
const canAdd = computed(() => !!props.newMcpServer.name.trim() && !!props.newMcpServer.url.trim());

const COLUMNS: DataTableColumn<McpServer>[] = [
  { key: "name", label: "Server", value: (s) => s.name },
  { key: "url", label: "URL" },
  { key: "status", label: "Status" },
  { key: "actions", label: "Actions" },
];
</script>

<template>
  <div class="flex flex-col gap-4">
    <Panel title="Add a server">
      <form class="grid items-end gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto]" @submit.prevent="canAdd && $emit('add')">
        <FormField label="Name">
          <template #default="{ id }">
            <input :id="id" v-model="localName" type="text" class="form-control" placeholder="my-server" autocomplete="off" />
          </template>
        </FormField>
        <FormField label="URL">
          <template #default="{ id }">
            <input :id="id" v-model="localUrl" type="text" class="form-control font-mono" placeholder="http://127.0.0.1:8931/mcp" autocomplete="off" />
          </template>
        </FormField>
        <BaseButton type="submit" variant="primary" icon="plus" :disabled="!canAdd">Add server</BaseButton>
      </form>
    </Panel>

    <Panel title="MCP servers" flush>
      <DataTable
        :columns="COLUMNS"
        :rows="mcpServers"
        :row-key="(s) => s.name"
        caption="MCP servers"
        empty-title="No MCP servers"
        empty-body="Add a server above to give agents its tools."
      >
        <template #cell-url="{ row }">
          <span class="break-all font-mono text-[length:var(--text-small)] text-muted">{{ row.url }}</span>
        </template>
        <template #cell-status="{ row }">
          <StatusTag :state="row.enabled ? 'success' : 'neutral'" :label="row.enabled ? 'Enabled' : 'Disabled'" />
        </template>
        <template #cell-actions="{ row }">
          <span class="inline-flex items-center gap-2">
            <BaseToggle :model-value="row.enabled" :label="`Enable ${row.name}`" hide-label @update:model-value="$emit('toggle', row)" />
            <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Remove ${row.name}`" @click="$emit('remove', row.name)" />
          </span>
        </template>
      </DataTable>
    </Panel>
  </div>
</template>
