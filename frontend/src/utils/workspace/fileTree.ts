import type { FileTreeNode, TreeEntry, WorkspaceTree } from '../../types/workspace'

const SEPARATOR = '/'

function compareNodes(a: FileTreeNode, b: FileTreeNode): number {
  if (a.type !== b.type) return a.type === 'dir' ? -1 : 1
  return a.name.localeCompare(b.name)
}

function sortRecursively(nodes: FileTreeNode[]): FileTreeNode[] {
  nodes.sort(compareNodes)
  for (const node of nodes) sortRecursively(node.children)
  return nodes
}

/**
 * Builds the nested tree from the server's flat, path-sorted entries —
 * directories first, then by name, at every level. A parent missing from the
 * listing (possible when the server truncated) is created as a directory.
 */
export function buildFileTree(entries: TreeEntry[]): FileTreeNode[] {
  const roots: FileTreeNode[] = []
  const dirs = new Map<string, FileTreeNode>()

  function dirNode(path: string): FileTreeNode {
    const existing = dirs.get(path)
    if (existing) return existing
    const node = createNode(path, 'dir', false)
    dirs.set(path, node)
    return node
  }

  function createNode(path: string, type: TreeEntry['type'], collapsed: boolean): FileTreeNode {
    const cut = path.lastIndexOf(SEPARATOR)
    const node: FileTreeNode = { name: path.slice(cut + 1), path, type, collapsed, children: [] }
    const siblings = cut === -1 ? roots : dirNode(path.slice(0, cut)).children
    siblings.push(node)
    return node
  }

  for (const entry of entries) {
    if (entry.type === 'dir') {
      dirNode(entry.path).collapsed = entry.collapsed ?? false
    } else {
      createNode(entry.path, 'file', false)
    }
  }
  return sortRecursively(roots)
}

/** The tree's file paths, sorted — e.g. for a task-file picker. */
export function treeFilePaths(tree: WorkspaceTree): string[] {
  return tree.entries
    .filter((entry) => entry.type === 'file')
    .map((entry) => entry.path)
    .sort()
}

/**
 * The entries a filter shows: every entry without a query, otherwise the files
 * whose path contains it (ignoring case), at most `limit` of them — rendering
 * thousands of expanded rows blocks the page (measured, Phase 6). `total` is
 * the full match count, so the UI can say how many it left out.
 */
export function filterTreeEntries(entries: TreeEntry[], query: string, limit: number): { entries: TreeEntry[]; total: number } {
  const q = query.trim().toLowerCase()
  if (!q) return { entries, total: entries.length }
  const matches = entries.filter((e) => e.type === 'file' && e.path.toLowerCase().includes(q))
  return { entries: matches.slice(0, limit), total: matches.length }
}

/** Whether `path` is `base` itself or lies inside the folder `base`. */
export function isPathWithin(path: string, base: string): boolean {
  return path === base || path.startsWith(base + SEPARATOR)
}

/**
 * The paths a delete actually has to send: a path inside a folder that is
 * itself being deleted is dropped (the folder takes it along). Order is kept.
 */
export function withoutNestedPaths(paths: string[]): string[] {
  return paths.filter((path) => !paths.some((other) => other !== path && isPathWithin(path, other)))
}

/** Whether a folder above `path` is in `selected` — the delete takes `path` along with it. */
export function isCoveredBySelection(path: string, selected: ReadonlySet<string>): boolean {
  return [...selected].some((other) => other !== path && isPathWithin(path, other))
}

/** Whether something strictly inside the folder `path` is in `selected`. */
export function hasSelectionWithin(path: string, selected: ReadonlySet<string>): boolean {
  return [...selected].some((other) => other !== path && isPathWithin(other, path))
}

/** The workspace's top-level entries — what "delete all files" removes. */
export function topLevelPaths(entries: TreeEntry[]): string[] {
  return entries.filter((entry) => !entry.path.includes(SEPARATOR)).map((entry) => entry.path)
}
