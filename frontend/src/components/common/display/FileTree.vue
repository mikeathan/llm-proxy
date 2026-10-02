<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue"
import type { FileTreeNode } from "../../../types/workspace"
import Icon from "../../icons/Icon.vue"

// Workspace file tree (plan Phase 3). Only the rows of expanded folders are
// rendered (reveal-on-expand), as a flat list of ARIA tree items with levels,
// so a large tree costs what is on screen. Keyboard: arrows move, Right/Left
// expand, collapse or go to the parent, Home/End jump, Enter/Space open.
// Heavy folders (node_modules, …) are listed but never expanded.

const props = defineProps<{
  nodes: FileTreeNode[]
  selectedPath: string | null
  label: string
  truncated?: boolean
  /** Show every folder open (e.g. while the list is filtered). */
  expandAll?: boolean
}>()

const emit = defineEmits<{ (e: "select", path: string): void }>()

defineSlots<{
  /** Inline controls at the start of a row (e.g. a selection checkbox). */
  leading?(props: { node: FileTreeNode }): unknown
  /** Inline controls at the end of a file or folder row (e.g. delete). */
  actions?(props: { node: FileTreeNode }): unknown
  /** Content under a row (e.g. an inline delete confirmation). */
  below?(props: { node: FileTreeNode }): unknown
}>()

const SEPARATOR = "/"
const INDENT_REM = 0.75

interface Row {
  node: FileTreeNode
  level: number
  setSize: number
  posInSet: number
  parent: string | null
}

const expanded = ref(new Set<string>())
const focusedPath = ref<string | null>(null)
const items = new Map<string, HTMLElement>()

const isExpandable = (node: FileTreeNode) => node.type === "dir" && !node.collapsed
const isOpen = (path: string) => props.expandAll || expanded.value.has(path)

const rows = computed<Row[]>(() => {
  const out: Row[] = []
  const walk = (nodes: FileTreeNode[], level: number, parent: string | null) => {
    nodes.forEach((node, index) => {
      out.push({ node, level, setSize: nodes.length, posInSet: index + 1, parent })
      if (isExpandable(node) && isOpen(node.path)) walk(node.children, level + 1, node.path)
    })
  }
  walk(props.nodes, 1, null)
  return out
})

// The selected file's folders open, so a deep link shows where the file is.
watch(
  () => props.selectedPath,
  (path) => {
    if (!path) return
    const segments = path.split(SEPARATOR)
    const next = new Set(expanded.value)
    for (let i = 1; i < segments.length; i++) next.add(segments.slice(0, i).join(SEPARATOR))
    expanded.value = next
    focusedPath.value = path
  },
  { immediate: true },
)

// One row is in the tab order (roving tabindex): the focused, else the first.
const tabStop = computed(() => {
  const paths = rows.value.map((row) => row.node.path)
  return focusedPath.value && paths.includes(focusedPath.value) ? focusedPath.value : paths[0] ?? null
})

function setExpanded(path: string, open: boolean) {
  const next = new Set(expanded.value)
  if (open) next.add(path)
  else next.delete(path)
  expanded.value = next
}

function activate(node: FileTreeNode) {
  focusedPath.value = node.path
  if (node.type === "file") emit("select", node.path)
  else if (isExpandable(node)) setExpanded(node.path, !expanded.value.has(node.path))
}

async function focusRow(path: string | null | undefined) {
  if (!path) return
  focusedPath.value = path
  await nextTick()
  items.get(path)?.focus()
}

function onKeydown(event: KeyboardEvent, index: number) {
  // Keys pressed on a row's own controls (a delete button) are theirs.
  if (event.target !== event.currentTarget) return
  const row = rows.value[index]
  if (!row) return
  const { node } = row
  const open = isOpen(node.path)
  const handlers: Record<string, () => void> = {
    ArrowDown: () => void focusRow(rows.value[index + 1]?.node.path),
    ArrowUp: () => void focusRow(rows.value[index - 1]?.node.path),
    Home: () => void focusRow(rows.value[0]?.node.path),
    End: () => void focusRow(rows.value[rows.value.length - 1]?.node.path),
    ArrowRight: () => {
      if (!isExpandable(node)) return
      if (!open) setExpanded(node.path, true)
      else void focusRow(node.children[0]?.path)
    },
    ArrowLeft: () => {
      if (isExpandable(node) && open) setExpanded(node.path, false)
      else void focusRow(row.parent)
    },
    Enter: () => activate(node),
    " ": () => activate(node),
  }
  const handler = handlers[event.key]
  if (!handler) return
  event.preventDefault()
  handler()
}

function setItemRef(path: string, el: unknown) {
  if (el instanceof HTMLElement) items.set(path, el)
  else items.delete(path)
}
</script>

<template>
  <div class="flex flex-col">
    <ul v-if="nodes.length" role="tree" :aria-label="label" class="flex flex-col py-1">
      <li
        v-for="(row, index) in rows"
        :key="row.node.path"
        :ref="(el) => setItemRef(row.node.path, el)"
        role="treeitem"
        :data-path="row.node.path"
        :aria-level="row.level"
        :aria-setsize="row.setSize"
        :aria-posinset="row.posInSet"
        :aria-expanded="isExpandable(row.node) ? isOpen(row.node.path) : undefined"
        :aria-selected="row.node.type === 'file' ? row.node.path === selectedPath : undefined"
        :tabindex="row.node.path === tabStop ? 0 : -1"
        class="group outline-none"
        @click="activate(row.node)"
        @keydown="onKeydown($event, index)"
        @focus="focusedPath = row.node.path"
      >
        <div
          :class="[
            'flex h-7 cursor-pointer items-center gap-1.5 pr-2 font-mono text-[length:var(--text-small)] transition-colors duration-fast group-focus-visible:ring-2 group-focus-visible:ring-inset',
            row.node.type === 'file' && row.node.path === selectedPath
              ? 'bg-surface-active text-primary shadow-[inset_2px_0_0_rgb(var(--accent-brand))]'
              : 'text-secondary hover:bg-surface-hover hover:text-primary',
          ]"
          :style="{ paddingLeft: `${0.5 + (row.level - 1) * INDENT_REM}rem` }"
        >
          <slot name="leading" :node="row.node" />
          <Icon
            v-if="isExpandable(row.node)"
            name="chevron-right"
            size="xs"
            :class-name="['flex-none text-muted transition-transform duration-fast', isOpen(row.node.path) ? 'rotate-90' : ''].join(' ')"
          />
          <span v-else class="w-3 flex-none" aria-hidden="true"></span>
          <Icon :name="row.node.type === 'dir' ? 'nav-workspaces' : 'document'" size="xs" class-name="flex-none text-muted" />
          <span class="min-w-0 flex-1 truncate">{{ row.node.name }}</span>
          <span v-if="row.node.collapsed" class="flex-none text-[length:var(--text-micro)] text-muted">not listed</span>
          <slot name="actions" :node="row.node" />
        </div>
        <slot name="below" :node="row.node" />
      </li>
    </ul>
    <p v-else class="px-3 py-2 text-[length:var(--text-small)] text-muted">No files yet.</p>
    <p v-if="truncated" role="status" class="px-3 py-2 text-[length:var(--text-micro)] text-muted">
      This workspace is large: deeper files are not shown.
    </p>
  </div>
</template>
