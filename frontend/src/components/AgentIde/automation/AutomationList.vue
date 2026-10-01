<script setup lang="ts">
import { computed, ref } from "vue"
import { useConfirm } from "../../../composables/ui/useConfirm"
import { toAutomation, toAutomationEdit, toAutomationNew } from "../../../router/routes"
import { automationStatus, triggerLabel } from "../../../utils/automation/automationDisplay"
import type { Automation } from "../../../types/dispatcher"
import type { ChoiceOption, DataTableColumn } from "../../../types/ui"
import DataTable from "../../common/display/DataTable.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import SearchInput from "../../common/forms/SearchInput.vue"
import SelectInput from "../../common/forms/SelectInput.vue"
import BaseButton from "../../common/buttons/BaseButton.vue"
import EmptyState from "../../common/feedback/EmptyState.vue"
import Icon from "../../icons/Icon.vue"

// Every automation across workspaces, managed as a fleet. A workspace runs one
// automation at a time: while one runs, the others in that workspace can be
// neither started, edited nor deleted.

const props = defineProps<{
  automations: Automation[]
  loading: boolean
}>()

const emit = defineEmits<{
  (e: "run", auto: Automation): void
  (e: "stop", auto: Automation): void
  (e: "delete", auto: Automation): void
  (e: "cancel-queued", auto: Automation): void
}>()

const ALL_WORKSPACES = ""
const search = ref("")
const workspace = ref(ALL_WORKSPACES)
const { confirm } = useConfirm()

const busyWorkspaces = computed(() => new Set(props.automations.filter((a) => a.is_running).map((a) => a.workspace)))
const lockedBy = (auto: Automation) => !auto.is_running && busyWorkspaces.value.has(auto.workspace)

const workspaceOptions = computed<ChoiceOption[]>(() => [
  { value: ALL_WORKSPACES, label: "All workspaces" },
  ...[...new Set(props.automations.map((a) => a.workspace))].sort().map((ws) => ({ value: ws, label: ws })),
])

const rows = computed(() => {
  const query = search.value.trim().toLowerCase()
  return props.automations
    .filter((a) => !workspace.value || a.workspace === workspace.value)
    .filter((a) => !query || [a.name, a.workspace, a.model ?? "", a.task_file].some((f) => f.toLowerCase().includes(query)))
    .sort((a, b) => a.workspace.localeCompare(b.workspace) || a.name.localeCompare(b.name))
})

async function remove(auto: Automation) {
  const ok = await confirm({
    title: `Delete ${auto.name}?`,
    message: "Its schedule stops. Past runs stay in Activity until you clear them.",
    type: "error",
    confirmText: "Delete automation",
  })
  if (ok) emit("delete", auto)
}

async function cancelQueued(auto: Automation) {
  const ok = await confirm({
    title: `Cancel the queued run of ${auto.name}?`,
    message: "The run leaves the queue and does not start. Its schedule is unchanged.",
    type: "warning",
    confirmText: "Cancel run",
    cancelText: "Keep it queued",
  })
  if (ok) emit("cancel-queued", auto)
}

const COLUMNS: DataTableColumn<Automation>[] = [
  { key: "name", label: "Automation" },
  { key: "trigger", label: "Schedule", value: triggerLabel },
  { key: "model", label: "Model", value: (a) => a.model || "default" },
  { key: "status", label: "Status" },
  { key: "actions", label: "Actions" },
]
const rowKey = (a: Automation) => a.id
</script>

<template>
  <div class="flex flex-col">
    <div class="flex flex-wrap items-center gap-2 px-4 py-3">
      <SelectInput v-model="workspace" :options="workspaceOptions" label="Workspace" />
      <SearchInput v-model="search" label="Search automations" class="min-w-[12rem] flex-1" />
    </div>

    <DataTable :columns="COLUMNS" :rows="rows" :row-key="rowKey" caption="Automations" :loading="loading && !props.automations.length">
      <template #empty>
        <EmptyState
          v-if="!props.automations.length"
          title="No automations yet"
          body="An automation runs a workspace task file on a schedule or on demand."
        >
          <template #action>
            <RouterLink :to="toAutomationNew()" class="font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline">Create an automation</RouterLink>
          </template>
        </EmptyState>
        <EmptyState v-else title="No automations match" body="Clear the search or pick another workspace." />
      </template>

      <template #cell-name="{ row }">
        <RouterLink :to="toAutomation(row.id)" class="block text-primary hover:underline focus-visible:outline-none focus-visible:ring-2">{{ row.name }}</RouterLink>
        <span class="block font-mono text-[length:var(--text-micro)] text-muted">{{ row.workspace }}</span>
      </template>
      <template #cell-status="{ row }">
        <StatusTag v-bind="automationStatus(row)" />
      </template>
      <template #cell-actions="{ row }">
        <span class="inline-flex items-center gap-1">
          <BaseButton v-if="row.is_running" variant="ghost" size="sm" icon="stop" icon-only :label="`Stop ${row.name}`" @click="emit('stop', row)" />
          <BaseButton v-else variant="ghost" size="sm" icon="play" icon-only :label="`Run ${row.name}`" :disabled="lockedBy(row)" @click="emit('run', row)" />
          <BaseButton v-if="row.queued && !row.is_running" variant="ghost" size="sm" icon="close" icon-only :label="`Cancel queued run of ${row.name}`" @click="cancelQueued(row)" />
          <RouterLink
            v-if="!lockedBy(row) && !row.is_running"
            :to="toAutomationEdit(row.id)"
            :aria-label="`Edit ${row.name}`"
            :title="`Edit ${row.name}`"
            class="inline-flex h-[26px] w-[26px] items-center justify-center rounded-[var(--radius-sm)] text-muted hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2"
          ><Icon name="edit" size="sm" /></RouterLink>
          <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete ${row.name}`" :disabled="lockedBy(row) || !!row.is_running" @click="remove(row)" />
        </span>
      </template>
    </DataTable>
  </div>
</template>
