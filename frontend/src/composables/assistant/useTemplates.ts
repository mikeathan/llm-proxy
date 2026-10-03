import type { Ref } from "vue";
import type { Template } from "../../types/templates";
import { DispatcherService } from "../../services/automation/dispatcherService";
import { useToast } from "../useToast";
import { useConfirm } from "../ui/useConfirm";

// Playbook injection for the Workspaces playbooks section. The route decides
// when the library is shown; `openFile` navigates to a file, so the editor
// always shows what the URL names.
export function useTemplates(
  selectedWorkspace: Readonly<Ref<string | null>>,
  selectedFile: Ref<{ workspace: string; filename: string } | null>,
  fileContent: Ref<string>,
  fetchWorkspaceTree: (workspace: string) => Promise<void>,
  openFile: (workspace: string, filename: string) => Promise<unknown>,
) {
  const toast = useToast();
  const { confirm } = useConfirm();

  // What "New file" would replace: the file's text, UNREADABLE when it is listed but could not be read, or
  // null when there is nothing to lose. The backend reads a missing file as empty content (not an error), so
  // empty counts as nothing to replace; a failed read is not proof of absence, so the file list decides, and
  // when that fails too the error reaches the caller and nothing is written.
  const UNREADABLE = Symbol("unreadable");
  const existingContent = async (workspace: string, filename: string): Promise<string | typeof UNREADABLE | null> => {
    try {
      return (await DispatcherService.readWorkspaceFile(workspace, filename)) || null;
    } catch {
      const tree = await DispatcherService.listWorkspaceTree(workspace);
      return tree.entries.some((e) => e.type === "file" && e.path === filename) ? UNREADABLE : null;
    }
  };

  const handleInjectTemplate = async (template: Template, mode: 'append' | 'create') => {
    const workspace = selectedWorkspace.value;
    if (!workspace) {
      toast.error("Please select a workspace first");
      return;
    }

    // Append only into this workspace's open buffer; otherwise create a new file.
    const file = selectedFile.value?.workspace === workspace ? selectedFile.value : null;
    if (mode === 'create' || !file) {
      const filename = `${template.id}.md`;
      try {
        const existing = await existingContent(workspace, filename);
        if (existing === template.content) {
          // Already this exact playbook: nothing to replace.
          await openFile(workspace, filename);
          toast.success(`${filename} is already up to date`);
          return;
        }
        if (existing !== null) {
          const replace = await confirm({
            title: "Replace existing file?",
            message: `${filename} already exists in this workspace. Replacing it discards its current contents.`,
            confirmText: "Replace",
          });
          if (!replace) return;
        }
        await DispatcherService.writeWorkspaceFile(workspace, filename, template.content);
        await fetchWorkspaceTree(workspace);
        await openFile(workspace, filename);
        toast.success(existing === null ? `Created new playbook: ${filename}` : `Replaced ${filename}`);
      } catch (err) {
        console.error("Failed to auto-create playbook", err);
        toast.error("Failed to create file: " + err);
      }
      return;
    }

    // Otherwise append to the open buffer and return to it.
    const content = template.content;
    if (fileContent.value && !fileContent.value.endsWith("\n")) {
      fileContent.value += "\n\n";
    }
    fileContent.value += content;
    await openFile(file.workspace, file.filename);
    toast.success("Playbook added to editor - remember to save!");
  };

  return {
    handleInjectTemplate,
  };
}
