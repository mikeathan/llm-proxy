import { ref } from "vue"
import { DispatcherService } from "../../services/automation/dispatcherService"

export function useFileEditor(toast: { error: (msg: string) => void }) {
  const selectedFile = ref<{ workspace: string; filename: string } | null>(null)
  const fileContent = ref("")
  const loadingFile = ref(false)
  const savingFile = ref(false)

  async function handleOpenFile(workspace: string, filename: string) {
    selectedFile.value = { workspace, filename }
    loadingFile.value = true
    fileContent.value = ""
    try {
      const result = await DispatcherService.readWorkspaceFile(workspace, filename)
      fileContent.value = result
    } catch (err) {
      console.error("Error loading file", err)
      fileContent.value = "Error loading file content."
    } finally {
      loadingFile.value = false
    }
  }

  async function handleSaveFile(onSaved?: () => void) {
    if (!selectedFile.value) return
    savingFile.value = true
    try {
      await DispatcherService.writeWorkspaceFile(
        selectedFile.value.workspace,
        selectedFile.value.filename,
        fileContent.value,
      )
      if (selectedFile.value.filename === "config.yaml") {
        onSaved?.()
      }
    } catch (err) {
      console.error("Error saving file", err)
      toast.error("Error saving file: " + err)
    } finally {
      savingFile.value = false
    }
  }

  async function handleCreateFile(workspace: string, filename: string, onCreated?: (ws: string) => void) {
    try {
      await DispatcherService.writeWorkspaceFile(workspace, filename, "")
      onCreated?.(workspace)
    } catch (err) {
      console.error("Error creating file", err)
      toast.error("Error creating file: " + err)
    }
  }

  function closeFile() {
    selectedFile.value = null
    fileContent.value = ""
  }

  return {
    selectedFile,
    fileContent,
    loadingFile,
    savingFile,
    handleOpenFile,
    handleSaveFile,
    handleCreateFile,
    closeFile,
  }
}
