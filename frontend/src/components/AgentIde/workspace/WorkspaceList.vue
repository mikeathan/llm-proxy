<script setup lang="ts">
import { ref } from "vue"
import { useConfirm } from "../../../composables/ui/useConfirm"
import { toWorkspace } from "../../../router/routes"
import { RESOURCE_NAME_PATTERN, RESOURCE_NAME_RULE } from "../../../constants/validation"
import type { DataTableColumn } from "../../../types/ui"
import DataTable from "../../common/display/DataTable.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import BaseButton from "../../common/buttons/BaseButton.vue"
import EmptyState from "../../common/feedback/EmptyState.vue"

// Every workspace, with create and delete. Names follow the backend's rule
// (letters, digits, dashes, underscores; up to 64), checked before sending.

interface WorkspaceRow {
  id: string
}

const props = defineProps<{
  workspaces: WorkspaceRow[]
  externalAccess: Record<string, boolean>
  loading: boolean
}>()

const emit = defineEmits<{
  (e: "create", name: string): void
  (e: "delete", id: string): void
}>()

const WORKSPACE_NAME = RESOURCE_NAME_PATTERN
const NAME_RULE = RESOURCE_NAME_RULE

const newName = ref("")
const nameError = ref("")
const { confirm } = useConfirm()

function create() {
  const name = newName.value.trim()
  if (!WORKSPACE_NAME.test(name)) {
    nameError.value = NAME_RULE
    return
  }
  nameError.value = ""
  newName.value = ""
  emit("create", name)
}

async function remove(id: string) {
  const ok = await confirm({
    title: `Delete workspace ${id}?`,
    message: "Its files, memories, conversations and run history are removed. This cannot be undone.",
    type: "error",
    confirmText: "Delete workspace",
  })
  if (ok) emit("delete", id)
}

const COLUMNS: DataTableColumn<WorkspaceRow>[] = [
  { key: "name", label: "Workspace" },
  { key: "access", label: "Network" },
  { key: "actions", label: "Actions" },
]
const rowKey = (row: WorkspaceRow) => row.id
</script>

<template>
  <div class="flex flex-col gap-3">
    <form class="flex flex-wrap items-start gap-2 px-4 pt-4" @submit.prevent="create">
      <label class="flex min-w-[14rem] flex-1 flex-col gap-1.5">
        <span class="text-[length:var(--text-small)] font-medium text-secondary">New workspace</span>
        <input
          v-model="newName"
          name="workspace-name"
          placeholder="e.g. home-lab"
          :aria-invalid="nameError ? 'true' : undefined"
          aria-describedby="workspace-name-rule"
          class="h-8 w-full rounded-[var(--radius-md)] border bg-canvas px-2.5 font-mono text-[length:var(--text-small)] text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2"
          :class="nameError ? 'border-state-error' : 'border-control'"
        />
        <span v-if="nameError" role="alert" class="text-[length:var(--text-small)] text-state-error">{{ nameError }}</span>
        <span v-else id="workspace-name-rule" class="text-[length:var(--text-small)] text-faint">{{ NAME_RULE }}</span>
      </label>
      <BaseButton type="submit" icon="plus" class-name="mt-[1.6rem]">Create</BaseButton>
    </form>

    <DataTable :columns="COLUMNS" :rows="props.workspaces" :row-key="rowKey" caption="Workspaces" :loading="loading && !props.workspaces.length">
      <template #empty>
        <EmptyState title="No workspaces yet" body="A workspace holds task files, playbooks, memories and conversations for one agent. Create one above." />
      </template>
      <template #cell-name="{ row }">
        <RouterLink :to="toWorkspace(row.id)" class="font-mono text-primary hover:underline focus-visible:outline-none focus-visible:ring-2">{{ row.id }}</RouterLink>
      </template>
      <template #cell-access="{ row }">
        <StatusTag v-if="externalAccess[row.id]" state="info" label="External access" />
        <span v-else class="text-faint">Workspace only</span>
      </template>
      <template #cell-actions="{ row }">
        <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete workspace ${row.id}`" @click="remove(row.id)" />
      </template>
    </DataTable>
  </div>
</template>
