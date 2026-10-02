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
import SelectionBar from "../../common/forms/SelectionBar.vue"
import BaseButton from "../../common/buttons/BaseButton.vue"
import EmptyState from "../../common/feedback/EmptyState.vue"
import Icon from "../../icons/Icon.vue"

// Every automation across workspaces, managed as a fleet. A workspace runs one
// automation at a time: while one runs, the others in that workspace can be
// neither started, edited nor deleted. Select mode picks several to delete
// together; running and locked rows cannot be picked, and only the rows the
// filter shows are ever deleted.

const props = defineProps<{
  automations: Automation[]
  loading: boolean
}>()

const emit = defineEmits<{
  (e: "run", auto: Automation): void
  (e: "stop", auto: Automation): void
  (e: "delete", auto: Automation): void
  (e: "delete-many", autos: Automation[]): void
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

// ── Select mode ──────────────────────────────────────────────────────────
const MAX_NAMED_IN_CONFIRM = 5
const selecting = ref(false)
const selected = ref(new Set<string>())

const isDeletable = (auto: Automation) => !auto.is_running && !lockedBy(auto)
const deletableRows = computed(() => rows.value.filter(isDeletable))
// The ticked rows still on screen: a row the filter hides drops out of the pick.
const picked = computed(() => deletableRows.value.filter((a) => selected.value.has(a.id)))
const allSelected = computed(() => deletableRows.value.length > 0 && picked.value.length === deletableRows.value.length)
const someSelected = computed(() => picked.value.length > 0 && !allSelected.value)

function stopSelecting() {
  selecting.value = false
  selected.value = new Set()
}

function toggle(auto: Automation, on: boolean) {
  const next = new Set(selected.value)
  if (on) next.add(auto.id)
  else next.delete(auto.id)
  selected.value = next
}

function toggleAll(on: boolean) {
  selected.value = on ? new Set(deletableRows.value.map((a) => a.id)) : new Set()
}

function describeNames(names: string[]): string {
  const more = names.length - MAX_NAMED_IN_CONFIRM
  const named = names.slice(0, MAX_NAMED_IN_CONFIRM).join(", ")
  return more > 0 ? `${named} and ${more} more` : named
}

async function removeSelected() {
  const autos = picked.value
  if (!autos.length) return
  const ok = await confirm({
    title: `Delete ${autos.length} automation${autos.length === 1 ? "" : "s"}?`,
    message: `${describeNames(autos.map((a) => a.name))}. Their schedules stop. Past runs stay in Activity until you clear them.`,
    type: "error",
    confirmText: "Delete selected",
  })
  if (!ok) return
  emit("delete-many", autos)
  stopSelecting()
}

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

const SELECT_COLUMN: DataTableColumn<Automation> = { key: "select", label: "Select" }
const ACTIONS_COLUMN: DataTableColumn<Automation> = { key: "actions", label: "Actions" }
const FIELD_COLUMNS: DataTableColumn<Automation>[] = [
  { key: "name", label: "Automation" },
  { key: "trigger", label: "Schedule", value: triggerLabel },
  { key: "model", label: "Model", value: (a) => a.model || "default" },
  { key: "status", label: "Status" },
]
// Select mode trades the row actions for a checkbox column.
const columns = computed(() => (selecting.value ? [SELECT_COLUMN, ...FIELD_COLUMNS] : [...FIELD_COLUMNS, ACTIONS_COLUMN]))
const rowKey = (a: Automation) => a.id
</script>

<template>
  <div class="flex flex-col">
    <div class="flex flex-wrap items-center gap-2 px-4 py-3">
      <SelectInput v-model="workspace" :options="workspaceOptions" label="Workspace" />
      <SearchInput v-model="search" label="Search automations" class="min-w-[12rem] flex-1" />
      <BaseButton v-if="rows.length && !selecting" variant="secondary" size="sm" aria-label="Select automations" @click="selecting = true">Select</BaseButton>
    </div>
    <SelectionBar
      v-if="selecting"
      class="mx-4 mb-3"
      :count="picked.length"
      :all-selected="allSelected"
      :some-selected="someSelected"
      @toggle-all="toggleAll"
      @delete="removeSelected"
      @done="stopSelecting"
    />

    <DataTable :columns="columns" :rows="rows" :row-key="rowKey" caption="Automations" :loading="loading && !props.automations.length">
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

      <template #cell-select="{ row }">
        <input
          type="checkbox"
          :aria-label="`Select ${row.name}`"
          :title="isDeletable(row) ? undefined : `${row.name} cannot be deleted while ${row.is_running ? 'it is running' : 'another automation runs in this workspace'}`"
          :checked="selected.has(row.id) && isDeletable(row)"
          :disabled="!isDeletable(row)"
          class="h-5 w-5 cursor-pointer focus-visible:outline-none focus-visible:ring-2 disabled:cursor-not-allowed sm:h-3.5 sm:w-3.5"
          @change="toggle(row, ($event.target as HTMLInputElement).checked)"
        />
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
