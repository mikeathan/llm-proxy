import { computed, type Ref } from "vue"
import type { AutomationRun } from "../../types/dispatcher"
import type { MemoryEntry } from "../../types/memory"
import type { WorkspaceLocation } from "../../types/routes"
import type { WorkspaceMainView } from "../../types/ui"

/**
 * Derives the Workspaces layout. What is addressable — workspace, file,
 * section, assistant — comes from the route (plan D18); the view manager adds
 * only the selections that are not: a run opened from the history list and a
 * memory entry. Below `lg` the view shows one pane: the explorer on the
 * workspace overview, the addressed page otherwise.
 */
export function useViewManager(deps: {
  location: Readonly<Ref<WorkspaceLocation>>
  selectedRun: Ref<AutomationRun | null>
  selectedMemory: Ref<MemoryEntry | null>
  isMobile: Readonly<Ref<boolean>>
}) {
  const activeMainView = computed<WorkspaceMainView>(() => {
    const at = deps.location.value
    if (deps.selectedRun.value) return "history"
    if (at.ws && at.assistant) return "assistant"
    if (at.section === "settings") return "settings"
    if (at.section === "playbooks") return "playbooks"
    if (at.section === "memory") return deps.selectedMemory.value ? "memory-detail" : "memory"
    if (at.filePath) return "editor"
    return "overview"
  })

  const explorerVisible = computed(() => !deps.isMobile.value || activeMainView.value === "overview")
  const mainVisible = computed(() => !deps.isMobile.value || activeMainView.value !== "overview")

  return { activeMainView, explorerVisible, mainVisible }
}
