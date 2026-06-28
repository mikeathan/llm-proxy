import { ref, computed, type Ref } from "vue"
import type { Automation, AutomationRun, RecordingMeta } from "../../types/dispatcher"

export function useAutomationRunner(
  automations: Ref<Automation[]>,
  triggerAutomation: (workspace: string, name: string, recordingRef?: string) => Promise<void>,
  stopAutomation: (workspace: string) => Promise<void>,
  refreshHistory: () => Promise<void>,
  fetchAutomations: (silent?: boolean) => Promise<void>,
) {
  const selectedAutomationId = ref<string | null>(null)
  const selectedRun = ref<AutomationRun | null>(null)
  const triggering = ref(false)
  const lastTriggerResult = ref<string | null>(null)

  const selectedAutomation = computed(() => {
    if (!selectedAutomationId.value) return null
    return (
      automations.value.find((a) => a.id === selectedAutomationId.value) || null
    )
  })

  const anyRunningInSelectedWorkspace = computed(() => {
    if (!selectedAutomation.value) return false
    const workspace = selectedAutomation.value.workspace
    return automations.value.some((a) => a.workspace === workspace && a.is_running)
  })

  function findAutomationForRun(run: AutomationRun): Automation | undefined {
    return automations.value.find(
      (a) => a.name === run.automation_name && a.workspace === run.workspace_id,
    )
  }

  function selectAutomation(auto: Automation) {
    selectedAutomationId.value = auto.id
    selectedRun.value = null
    lastTriggerResult.value = null
  }

  function selectRun(run: AutomationRun) {
    const auto = findAutomationForRun(run)
    if (auto) {
      selectedAutomationId.value = auto.id
      selectedRun.value = run
      lastTriggerResult.value = null
    } else {
      selectedRun.value = run
      selectedAutomationId.value = null
    }
  }

  async function handleTrigger() {
    if (!selectedAutomation.value) return
    triggering.value = true
    lastTriggerResult.value = `Running ${selectedAutomation.value.name}...`
    try {
      await triggerAutomation(
        selectedAutomation.value.workspace,
        selectedAutomation.value.name,
      )
      lastTriggerResult.value = `Triggered ${selectedAutomation.value.name} successfully`
    } catch {
      lastTriggerResult.value = `Failed to trigger ${selectedAutomation.value.name}`
    } finally {
      triggering.value = false
      await fetchAutomations()
      await refreshHistory()
    }
  }

  async function handleReplayRecording(auto: Automation, recording: RecordingMeta) {
    selectedAutomationId.value = auto.id
    triggering.value = true
    lastTriggerResult.value = `Replaying recording for ${auto.name}...`
    try {
      await triggerAutomation(auto.workspace, auto.name, recording.id)
      lastTriggerResult.value = `Replayed ${auto.name} from recording`
    } catch {
      lastTriggerResult.value = `Failed to replay ${auto.name}`
    } finally {
      triggering.value = false
      await fetchAutomations()
      await refreshHistory()
    }
  }

  async function handleStopRecording(workspace: string) {
    try {
      await stopAutomation(workspace)
      lastTriggerResult.value = "Recording replay stopped"
    } catch (err) {
      console.error("Stop recording replay failed", err)
    } finally {
      await fetchAutomations()
    }
  }

  function handleShowAutomation(id: string) {
    selectedAutomationId.value = id
  }

  async function handleStop() {
    if (!selectedAutomation.value) return
    try {
      await stopAutomation(selectedAutomation.value.workspace)
      lastTriggerResult.value = `Stopped ${selectedAutomation.value.name}`
    } catch (err) {
      console.error("Stop failed", err)
    } finally {
      await fetchAutomations()
    }
  }

  function clearSelection() {
    selectedAutomationId.value = null
    selectedRun.value = null
    lastTriggerResult.value = null
  }

  return {
    selectedAutomationId,
    selectedRun,
    triggering,
    lastTriggerResult,
    selectedAutomation,
    anyRunningInSelectedWorkspace,
    selectAutomation,
    selectRun,
    findAutomationForRun,
    handleTrigger,
    handleReplayRecording,
    handleStopRecording,
    handleShowAutomation,
    handleStop,
    clearSelection,
  }
}
