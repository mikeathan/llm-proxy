<script setup lang="ts">
import { computed, ref } from "vue"
import { useConfirm } from "../../../composables/ui/useConfirm"
import { buildFileTree, filterTreeEntries } from "../../../utils/workspace/fileTree"
import { isSafeRelativePath } from "../../../utils/workspace/filePath"
import type { WorkspaceTree } from "../../../types/workspace"
import FileTree from "../../common/display/FileTree.vue"
import SearchInput from "../../common/forms/SearchInput.vue"
import BaseButton from "../../common/buttons/BaseButton.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"

// A workspace's files: filter, create (nested paths allowed), open, delete
// (confirmed). The tree reveals folders on demand; a filter opens them all.

const props = defineProps<{
  workspace: string
  tree: WorkspaceTree | undefined
  selectedPath: string | null
  loading: boolean
}>()

const emit = defineEmits<{
  (e: "open-file", path: string): void
  (e: "create-file", path: string): void
  (e: "delete-file", path: string): void
}>()

const PATH_RULE = "Use a path inside the workspace, such as notes/today.md."
// Matching files a filter shows at most: past this, rendering the expanded
// tree blocked the page (measured at the 5000-entry cap, Phase 6).
const MAX_FILTER_MATCHES = 200

const filter = ref("")
const newPath = ref("")
const pathError = ref("")
const { confirm } = useConfirm()

const filtered = computed(() => filterTreeEntries(props.tree?.entries ?? [], filter.value, MAX_FILTER_MATCHES))
const nodes = computed(() => buildFileTree(filtered.value.entries))
const hiddenMatches = computed(() => (filter.value.trim() ? filtered.value.total - filtered.value.entries.length : 0))

function create() {
  const path = newPath.value.trim()
  if (!isSafeRelativePath(path)) {
    pathError.value = PATH_RULE
    return
  }
  pathError.value = ""
  newPath.value = ""
  emit("create-file", path)
}

async function remove(path: string) {
  const ok = await confirm({
    title: `Delete ${path}?`,
    message: "The file is removed from the workspace. This cannot be undone.",
    type: "error",
    confirmText: "Delete file",
  })
  if (ok) emit("delete-file", path)
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-2">
    <SearchInput v-model="filter" label="Filter files" />
    <p v-if="hiddenMatches > 0" role="status" class="m-0 text-[length:var(--text-small)] text-muted">
      Showing {{ MAX_FILTER_MATCHES }} of {{ filtered.total }} matches — keep typing to narrow it.
    </p>
    <form class="flex flex-col gap-1" @submit.prevent="create">
      <span class="flex gap-1.5">
        <input
          v-model="newPath"
          name="new-file"
          aria-label="New file path"
          placeholder="New file, e.g. notes/today.md"
          :aria-invalid="pathError ? 'true' : undefined"
          class="h-8 min-w-0 flex-1 rounded-[var(--radius-md)] border bg-canvas px-2.5 font-mono text-[length:var(--text-small)] text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2"
          :class="pathError ? 'border-state-error' : 'border-control'"
        />
        <BaseButton type="submit" variant="secondary" icon="plus" icon-only label="Create file" />
      </span>
      <span v-if="pathError" role="alert" class="text-[length:var(--text-small)] text-state-error">{{ pathError }}</span>
    </form>

    <LoadingState v-if="loading && !tree" label="Loading files" :rows="4" />
    <div v-else class="-mx-2 min-h-0 flex-1 overflow-y-auto">
      <FileTree
        :nodes="nodes"
        :selected-path="selectedPath"
        :truncated="tree?.truncated ?? false"
        :expand-all="!!filter.trim()"
        :label="`Files in ${workspace}`"
        @select="emit('open-file', $event)"
      >
        <template #actions="{ node }">
          <span class="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100" @click.stop>
            <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete ${node.path}`" @click="remove(node.path)" />
          </span>
        </template>
      </FileTree>
    </div>
  </div>
</template>
