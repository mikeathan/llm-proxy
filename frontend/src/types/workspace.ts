// Workspace file tree (plan Phase 3). Mirrors the backend contract of
// GET /admin/api/dispatcher/workspaces/{ws}/tree (models.WorkspaceTree).

export type TreeEntryType = 'file' | 'dir'

export interface TreeEntry {
  /** Workspace-root-relative, slash-separated. */
  path: string
  type: TreeEntryType
  /** A heavy directory (node_modules, .venv, …) listed but not descended into. */
  collapsed?: boolean
}

export interface WorkspaceTree {
  entries: TreeEntry[]
  /** The server's entry cap cut off deeper entries. */
  truncated: boolean
}

/** One node of the rendered tree, built from the flat entries. */
export interface FileTreeNode {
  name: string
  path: string
  type: TreeEntryType
  collapsed: boolean
  children: FileTreeNode[]
}
