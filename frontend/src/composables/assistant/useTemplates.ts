import { ref } from "vue";
import type { Ref } from "vue";
import type { Template } from "../../types/templates";
import { DispatcherService } from "../../services/dispatcherService";
import { useToast } from "../useToast";

export function useTemplates(
  selectedWorkspace: Ref<string | null>,
  selectedFile: Ref<{ workspace: string; filename: string } | null>,
  fileContent: Ref<string>,
  fetchWorkspaceFiles: (workspace: string) => Promise<void>,
  handleOpenFile: (workspace: string, filename: string) => Promise<void>,
) {
  const showTemplates = ref(false);
  const toast = useToast();

  const handleInjectTemplate = async (template: Template, mode: 'append' | 'create') => {
    if (!selectedWorkspace.value) {
      toast.error("Please select a workspace first");
      return;
    }

    // If mode is 'create' OR no file is open, create a new one from template
    if (mode === 'create' || !selectedFile.value) {
      const filename = `${template.id}.md`;
      try {
        await DispatcherService.writeWorkspaceFile(
          selectedWorkspace.value,
          filename,
          template.content,
        );
        await fetchWorkspaceFiles(selectedWorkspace.value);
        await handleOpenFile(selectedWorkspace.value, filename);
        toast.success(`Created new playbook: ${filename}`);
      } catch (err) {
        console.error("Failed to auto-create playbook", err);
        toast.error("Failed to create file: " + err);
      }
      return;
    }

    // Otherwise append to current
    const content = template.content;
    if (fileContent.value && !fileContent.value.endsWith("\n")) {
      fileContent.value += "\n\n";
    }
    fileContent.value += content;
    toast.success("Playbook added to editor - remember to save!");
  };

  return {
    showTemplates,
    handleInjectTemplate,
  };
}
