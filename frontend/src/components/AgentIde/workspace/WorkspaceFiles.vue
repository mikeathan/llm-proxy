<script setup lang="ts">
import { computed, ref, watch } from "vue"
import { useConfirm } from "../../../composables/ui/useConfirm"
import { buildFileTree, filterTreeEntries, hasSelectionWithin, isCoveredBySelection, isPathWithin, topLevelPaths, withoutNestedPaths } from "../../../utils/workspace/fileTree"
import { isSafeRelativePath } from "../../../utils/workspace/filePath"
import type { FileTreeNode, WorkspaceTree } from "../../../types/workspace"
import FileTree from "../../common/display/FileTree.vue"
import SearchInput from "../../common/forms/SearchInput.vue"
import SelectionBar from "../../common/forms/SelectionBar.vue"
import BaseButton from "../../common/buttons/BaseButton.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"

// A workspace's files: filter, create (nested paths allowed), open, delete
// (confirmed). The tree reveals folders on demand; a filter opens them all.
// A folder is deleted with everything in it. Select mode picks several files
// and folders to delete together (ticking a folder ticks and locks everything
// in it); Select all picks the whole workspace.

const props = defineProps<{
  workspace: string
  tree: WorkspaceTree | undefined
  selectedPath: string | null
  loading: boolean
}>()

const emit = defineEmits<{
  (e: "open-file", path: string): void
  (e: "create-file", path: string): void
  (e: "delete-paths", paths: string[]): void
}>()

const PATH_RULE = "Use a path inside the workspace, such as notes/today.md."
const IRREVERSIBLE = "This cannot be undone."
// Matching files a filter shows at most: past this, rendering the expanded
// tree blocked the page (measured at the 5000-entry cap, Phase 6).
const MAX_FILTER_MATCHES = 200
// Names a selection confirm lists before summarising the rest.
const MAX_NAMED_IN_CONFIRM = 5

const filter = ref("")
const newPath = ref("")
const pathError = ref("")
const { confirm } = useConfirm()

const filtered = computed(() => filterTreeEntries(props.tree?.entries ?? [], filter.value, MAX_FILTER_MATCHES))
const nodes = computed(() => buildFileTree(filtered.value.entries))
const hiddenMatches = computed(() => (filter.value.trim() ? filtered.value.total - filtered.value.entries.length : 0))
const hasEntries = computed(() => (props.tree?.entries.length ?? 0) > 0)

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

async function remove(node: FileTreeNode) {
  const isDir = node.type === "dir"
  const ok = await confirm({
    title: `Delete ${node.path}?`,
    message: isDir
      ? `The folder and everything inside it are removed from the workspace. ${IRREVERSIBLE}`
      : `The file is removed from the workspace. ${IRREVERSIBLE}`,
    type: "error",
    confirmText: isDir ? "Delete folder" : "Delete file",
  })
  if (ok) emit("delete-paths", [node.path])
}

// ── Select mode ──────────────────────────────────────────────────────────
const selecting = ref(false)
const selected = ref(new Set<string>())
// What a delete will actually remove: the ticked paths minus anything a ticked
// folder already takes along, and how many entries sit inside those folders.
const selectedCount = computed(() => withoutNestedPaths([...selected.value]).length)
const insideCount = computed(() => {
  const roots = withoutNestedPaths([...selected.value])
  const entries = props.tree?.entries ?? []
  return entries.filter((e) => roots.some((root) => e.path !== root && isPathWithin(e.path, root))).length
})

function stopSelecting() {
  selecting.value = false
  selected.value = new Set()
}

// A selection belongs to one workspace's tree.
watch(() => props.workspace, stopSelecting)

function toggle(path: string, on: boolean) {
  const next = new Set(selected.value)
  if (on) next.add(path)
  else next.delete(path)
  selected.value = next
}

// Select all ticks every listed top-level entry, whatever the filter shows: the
// server lists breadth-first, so the top level is complete even when deeper
// files are cut. Folders carry their contents, so this is the whole workspace.
const topLevel = computed(() => topLevelPaths(props.tree?.entries ?? []))
const allSelected = computed(() => topLevel.value.length > 0 && topLevel.value.every((p) => selected.value.has(p)))
const someSelected = computed(() => selected.value.size > 0 && !allSelected.value)

function toggleAll(on: boolean) {
  selected.value = on ? new Set(topLevel.value) : new Set()
}

function describeSelection(paths: string[]): string {
  const named = paths.slice(0, MAX_NAMED_IN_CONFIRM).join(", ")
  const more = paths.length - MAX_NAMED_IN_CONFIRM
  return more > 0 ? `${named} and ${more} more` : named
}

async function removeSelected() {
  const paths = withoutNestedPaths([...selected.value])
  if (!paths.length) return
  const ok = await confirm({
    title: `Delete ${paths.length} selected item${paths.length === 1 ? "" : "s"}?`,
    message: `${describeSelection(paths)}. Folders are removed with everything inside them. ${IRREVERSIBLE}`,
    type: "error",
    confirmText: "Delete selected",
  })
  if (!ok) return
  emit("delete-paths", paths)
  stopSelecting()
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-2">
    <span class="flex items-center gap-1.5">
      <SearchInput v-model="filter" label="Filter files" class="min-w-0 flex-1" />
      <BaseButton v-if="hasEntries && !selecting" variant="secondary" size="sm" @click="selecting = true">Select</BaseButton>
    </span>
    <SelectionBar
      v-if="selecting"
      :count="selectedCount"
      :inside="insideCount"
      :all-selected="allSelected"
      :some-selected="someSelected"
      @toggle-all="toggleAll"
      @delete="removeSelected"
      @done="stopSelecting"
    />
    <p v-if="hiddenMatches > 0" role="status" class="m-0 text-[length:var(--text-small)] text-muted">
      Showing {{ MAX_FILTER_MATCHES }} of {{ filtered.total }} matches — keep typing to narrow it.
    </p>
    <form v-if="!selecting" class="flex flex-col gap-1" @submit.prevent="create">
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
        <template v-if="selecting" #leading="{ node }">
          <input
            type="checkbox"
            :aria-label="`Select ${node.path}`"
            :checked="selected.has(node.path) || isCoveredBySelection(node.path, selected)"
            :disabled="isCoveredBySelection(node.path, selected)"
            :indeterminate="node.type === 'dir' && !selected.has(node.path) && hasSelectionWithin(node.path, selected)"
            class="h-3.5 w-3.5 flex-none cursor-pointer focus-visible:outline-none focus-visible:ring-2"
            @click.stop
            @change="toggle(node.path, ($event.target as HTMLInputElement).checked)"
          />
        </template>
        <template v-if="!selecting" #actions="{ node }">
          <span class="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100" @click.stop>
            <BaseButton
              variant="ghost"
              size="sm"
              icon="trash"
              icon-only
              :label="node.type === 'dir' ? `Delete folder ${node.path}` : `Delete ${node.path}`"
              @click="remove(node)"
            />
          </span>
        </template>
      </FileTree>
    </div>
  </div>
</template>
