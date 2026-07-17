import { ref, computed, type Ref, type ComputedRef } from "vue"
import type { Automation, AutomationRun } from "../../types/dispatcher"
import type { MemoryEntry } from "../../types/memory"

export function useViewManager(deps: {
  selectedWorkspace: Ref<string | null>
  selectedFile: Ref<{ workspace: string; filename: string } | null>
  selectedRun: Ref<AutomationRun | null>
  selectedMemory: Ref<MemoryEntry | null>
  settingsWorkspaceId: Ref<string | null>
  selectedAutomation: ComputedRef<Automation | null>
  isMobile: Ref<boolean>
}) {
  const leftTab = ref<"explorer" | "automations" | "recordings" | "memory" | "activity">("explorer")
  const mobilePanel = ref<"explorer" | "workspace" | "monitor">("workspace")
  const workspaceMiddleTab = ref<"pulse" | "chat">("pulse")

  const memoryActive = computed(() => leftTab.value === "memory")

  const canOpenAssistant = computed(() => !!deps.selectedWorkspace.value)

  const activeMainView = computed(() => {
    if (deps.settingsWorkspaceId.value) return "workspace-settings"
    if (deps.selectedRun.value) return "history"
    if (deps.selectedMemory.value && deps.selectedWorkspace.value) return "memory-detail"
    if (deps.selectedFile.value) return "editor"
    if (deps.selectedWorkspace.value && workspaceMiddleTab.value === "chat") return "assistant"
    if (deps.selectedAutomation.value) return "automation"
    return "dashboard"
  })

  function toggleAssistant() {
    workspaceMiddleTab.value = workspaceMiddleTab.value === "chat" ? "pulse" : "chat"
    if (deps.isMobile.value) {
      mobilePanel.value = "workspace"
    }
  }

  function closeViewDetails() {
    workspaceMiddleTab.value = "pulse"
  }

  return {
    leftTab,
    mobilePanel,
    workspaceMiddleTab,
    activeMainView,
    memoryActive,
    canOpenAssistant,
    toggleAssistant,
    closeViewDetails,
  }
}
