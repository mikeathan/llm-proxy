<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  mcpServers: any[];
  newMcpServer: any;
}>();

const emit = defineEmits<{
  (e: "update:newMcpServer", server: any): void;
  (e: "addMCPServer"): void;
  (e: "toggleMCPServer", server: any): void;
  (e: "removeMCPServer", name: string): void;
}>();

const localName = computed({
  get: () => props.newMcpServer.name,
  set: (val) =>
    emit("update:newMcpServer", { ...props.newMcpServer, name: val }),
});

const localUrl = computed({
  get: () => props.newMcpServer.url,
  set: (val) =>
    emit("update:newMcpServer", { ...props.newMcpServer, url: val }),
});

function submitAddMCPServer() {
  emit("addMCPServer");
}
</script>
<template>
  <div class="mcp-container">
    <h2 class="mcp-title">MCP Servers</h2>

    <div class="mcp-form">
      <input
        v-model="localName"
        type="text"
        placeholder="Server Name"
        class="mcp-input"
      />
      <input
        v-model="localUrl"
        type="text"
        placeholder="Server URL"
        class="mcp-input"
      />
      <button @click="submitAddMCPServer" class="mcp-btn-add">Add</button>
    </div>

    <div class="mcp-table-container">
      <table class="mcp-table">
        <thead class="mcp-thead">
          <tr class="mcp-tr-head">
            <th class="mcp-th">Name</th>
            <th class="mcp-th">URL</th>
            <th class="mcp-th-right">Actions</th>
          </tr>
        </thead>
        <tbody class="mcp-tbody">
          <tr v-if="mcpServers.length === 0">
            <td colspan="3" class="mcp-td-empty">No MCP servers configured</td>
          </tr>
          <tr
            v-for="server in mcpServers"
            :key="server.name"
            class="mcp-tr-body"
          >
            <td class="mcp-td-name">
              <span
                :class="[
                  'mcp-status-dot',
                  server.enabled ? 'mcp-toggle-on' : 'mcp-toggle-off',
                ]"
              ></span>
              {{ server.name }}
            </td>
            <td class="mcp-td-url">{{ server.url }}</td>
            <td class="mcp-td-actions">
              <button
                @click="$emit('toggleMCPServer', server)"
                :class="[
                  'mcp-btn-toggle',
                  server.enabled ? 'btn-disable' : 'btn-enable',
                ]"
              >
                {{ server.enabled ? "Disable" : "Enable" }}
              </button>
              <button
                @click="$emit('removeMCPServer', server.name)"
                class="mcp-btn-delete"
              >
                Delete
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.mcp-container {
  @apply bg-gray-800 rounded-lg shadow border border-gray-700 p-6 flex flex-col;
}
.mcp-title {
  @apply text-lg font-semibold text-white mb-4 border-b border-gray-700 pb-2;
}
.mcp-form {
  @apply mb-5 flex flex-col sm:flex-row gap-2;
}
.mcp-input {
  @apply bg-gray-900 border border-gray-600 text-white rounded px-3 py-2 flex-1 text-sm focus:outline-none focus:border-blue-500;
}
.mcp-btn-add {
  @apply bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded text-sm font-medium transition-colors shrink-0;
}
.mcp-table-container {
  @apply overflow-x-auto flex-1 border border-gray-700 rounded-lg;
}
.mcp-table {
  @apply w-full text-left text-sm;
}
.mcp-thead {
  @apply bg-gray-900/80 sticky top-0;
}
.mcp-tr-head {
  @apply text-gray-400 border-b border-gray-700;
}
.mcp-th {
  @apply px-4 py-2 font-medium;
}
.mcp-th-right {
  @apply px-4 py-2 font-medium text-right;
}
.mcp-tbody {
  @apply divide-y divide-gray-700;
}
.mcp-td-empty {
  @apply px-4 py-6 text-center text-gray-500;
}
.mcp-tr-body {
  @apply hover:bg-gray-700/30;
}
.mcp-td-name {
  @apply px-4 py-3 text-white font-medium flex items-center gap-2;
}
.mcp-status-dot {
  @apply w-2 h-2 rounded-full;
}
.mcp-toggle-on {
  @apply bg-green-500;
}
.mcp-toggle-off {
  @apply bg-gray-600;
}
.mcp-td-url {
  @apply px-4 py-3 text-gray-400 font-mono text-xs;
}
.mcp-td-actions {
  @apply px-4 py-3 text-right flex justify-end gap-2;
}
.mcp-btn-toggle {
  @apply px-3 py-1 rounded text-xs font-medium transition-colors;
}
.btn-enable {
  @apply bg-blue-600 hover:bg-blue-500 text-white;
}
.btn-disable {
  @apply bg-gray-700 hover:bg-gray-600 text-gray-300;
}
.mcp-btn-delete {
  @apply text-red-400 hover:text-red-300 text-xs;
}
</style>
