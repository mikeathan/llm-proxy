import { ref } from "vue"
import type { TaskMode } from "../../types/automation"
import type { TemplateMetadata } from "../../types/templates"
import { TemplateService } from "../../services/template/templateService"
import { useWorkspaceFileWriter } from "../editor/useWorkspaceFileWriter"
import { useConfirm } from "../ui/useConfirm"

const TASK_FILE_EXTENSION = ".md"

/** The workspace file a written task is saved as: the automation's own name. */
export const taskFileName = (automationName: string): string => `${automationName}${TASK_FILE_EXTENSION}`

// Writing an automation's task inside the form: free text, optionally started from a
// playbook, saved as a workspace file when the automation is created. The playbook
// library is loaded once, on first use.
export function useTaskAuthoring() {
  const { writeFile } = useWorkspaceFileWriter()
  const { confirm } = useConfirm()

  const mode = ref<TaskMode>("existing")
  const content = ref("")
  const playbookId = ref("")
  const playbooks = ref<TemplateMetadata[]>([])
  const playbooksFailed = ref(false)

  let loaded = false
  // The text a playbook last put in the box: while the box still holds exactly it, nothing the user wrote is at risk.
  let playbookText = ""

  async function loadPlaybooks() {
    if (loaded) return
    try {
      playbooks.value = await TemplateService.listTemplates()
      playbooksFailed.value = false
      loaded = true
    } catch {
      playbooksFailed.value = true
    }
  }

  /** Puts the playbook's text in the box; false when the user declined to replace what they wrote. */
  async function choosePlaybook(id: string): Promise<boolean> {
    const template = await TemplateService.getTemplate(id)
    const written = content.value !== "" && content.value !== playbookText
    if (written) {
      const replace = await confirm({
        title: "Replace what you wrote?",
        message: "Starting from this playbook replaces the task text you have written.",
        confirmText: "Replace",
      })
      if (!replace) return false
    }
    content.value = template.content
    playbookText = template.content
    playbookId.value = id
    return true
  }

  /** Saves the written task as `<name>.md`; null when replacing an existing file was declined. */
  async function saveTask(workspace: string, automationName: string): Promise<string | null> {
    const file = taskFileName(automationName)
    const outcome = await writeFile(workspace, file, content.value)
    return outcome === "cancelled" ? null : file
  }

  function reset() {
    mode.value = "existing"
    content.value = ""
    playbookId.value = ""
    playbookText = ""
  }

  return { mode, content, playbookId, playbooks, playbooksFailed, loadPlaybooks, choosePlaybook, saveTask, reset }
}
