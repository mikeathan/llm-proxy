import type { FileWriteOutcome } from "../../types/workspace"
import { DispatcherService } from "../../services/automation/dispatcherService"
import { useConfirm } from "../ui/useConfirm"

// A file in a workspace that is listed but could not be read: it may hold content, so it is never overwritten unasked.
const UNREADABLE = Symbol("unreadable")

// Writing a workspace file without silently destroying one: the playbook library and the
// automation form both create files the user may already have.
export function useWorkspaceFileWriter() {
  const { confirm } = useConfirm()

  // What writing would replace: the file's text, UNREADABLE when it is listed but could not be read, or
  // null when there is nothing to lose. The backend reads a missing file as empty content (not an error), so
  // empty counts as nothing to replace; a failed read is not proof of absence, so the file list decides, and
  // when that fails too the error reaches the caller and nothing is written.
  const existingContent = async (workspace: string, filename: string): Promise<string | typeof UNREADABLE | null> => {
    try {
      return (await DispatcherService.readWorkspaceFile(workspace, filename)) || null
    } catch {
      const tree = await DispatcherService.listWorkspaceTree(workspace)
      return tree.entries.some((e) => e.type === "file" && e.path === filename) ? UNREADABLE : null
    }
  }

  async function writeFile(workspace: string, filename: string, content: string): Promise<FileWriteOutcome> {
    const existing = await existingContent(workspace, filename)
    if (existing === content) return "unchanged"
    if (existing !== null) {
      const replace = await confirm({
        title: "Replace existing file?",
        message: `${filename} already exists in this workspace. Replacing it discards its current contents.`,
        confirmText: "Replace",
      })
      if (!replace) return "cancelled"
    }
    await DispatcherService.writeWorkspaceFile(workspace, filename, content)
    return existing === null ? "created" : "replaced"
  }

  return { writeFile }
}
