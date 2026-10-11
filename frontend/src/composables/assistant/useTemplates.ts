import type { Ref } from "vue";
import type { Template } from "../../types/templates";
import { useToast } from "../useToast";
import type { FileWriteOutcome } from "../../types/workspace";
import { useWorkspaceFileWriter } from "../editor/useWorkspaceFileWriter";

const PLAYBOOK_TOAST: Record<Exclude<FileWriteOutcome, "cancelled">, (filename: string) => string> = {
  created: (filename) => `Created new playbook: ${filename}`,
  replaced: (filename) => `Replaced ${filename}`,
  unchanged: (filename) => `${filename} is already up to date`,
};

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
  const { writeFile } = useWorkspaceFileWriter();

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
        const outcome = await writeFile(workspace, filename, template.content);
        if (outcome === "cancelled") return;
        await fetchWorkspaceTree(workspace);
        await openFile(workspace, filename);
        toast.success(PLAYBOOK_TOAST[outcome](filename));
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
