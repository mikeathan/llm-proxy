import { ref } from "vue"
import type { AutomationRun } from "../../types/dispatcher"

export function useWorkspaceHistory() {
  const workspaceHistory = ref<AutomationRun[]>([])

  async function refreshHistory(
    selectedWorkspace: string | null,
    fetchWorkspaceState: (ws: string) => Promise<{ history: AutomationRun[] }>,
    fetchGlobalActivity: () => Promise<AutomationRun[]>,
  ) {
    try {
      if (selectedWorkspace) {
        const state = await fetchWorkspaceState(selectedWorkspace)
        workspaceHistory.value = state.history || []
      } else {
        const global = await fetchGlobalActivity()
        workspaceHistory.value = global || []
      }
    } catch (err) {
      console.error("Failed to fetch history", err)
    }
  }

  return {
    workspaceHistory,
    refreshHistory,
  }
}
