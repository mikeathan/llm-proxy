import { ref, computed, getCurrentScope, onScopeDispose, type Ref } from "vue"
import type { Automation, AutomationRun, RecordingMeta, TriggerResponse } from "../../types/dispatcher"

// The server answers a trigger as soon as the run is admitted, but marks the automation running a moment later (after the
// workspace lock and the pre-flight). Until it does, the page follows it once a second so the Run button shows "starting"
// instead of looking as if the click did nothing; it gives up after a minute.
const START_POLL_MS = 1000
const START_WAIT_MAX_MS = 60_000
const TRIGGER_STARTED = "started"

/** The newest run on an automation; a new one appearing means the run came and went without showing as running. */
const lastRunId = (a: Automation | null | undefined) => a?.history?.[a.history.length - 1]?.id ?? ""

export function useAutomationRunner(
  automations: Ref<Automation[]>,
  triggerAutomation: (workspace: string, name: string, recordingRef?: string) => Promise<TriggerResponse | void>,
  stopAutomation: (workspace: string) => Promise<void>,
  refreshHistory: () => Promise<void>,
  fetchAutomations: (silent?: boolean) => Promise<void>,
) {
  const selectedAutomationId = ref<string | null>(null)
  const selectedRun = ref<AutomationRun | null>(null)
  const triggering = ref(false)
  const lastTriggerResult = ref<string | null>(null)
  // True from the moment a run was admitted until the server shows it running (or it has visibly ended or failed to start).
  const starting = ref(false)
  let startWatch: ReturnType<typeof setInterval> | null = null

  function stopWatchingStart() {
    if (startWatch !== null) clearInterval(startWatch)
    startWatch = null
    starting.value = false
  }
  if (getCurrentScope()) onScopeDispose(stopWatchingStart)

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
    stopWatchingStart()
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

  /** Follows an admitted run until the server shows it running; a run that is queued or skipped has nothing to wait for. */
  function watchStart(outcome: TriggerResponse | void, baseline: string) {
    const auto = selectedAutomation.value
    if (outcome?.status !== TRIGGER_STARTED || !auto || auto.is_running || auto.queued || lastRunId(auto) !== baseline) return
    stopWatchingStart()
    starting.value = true
    const began = Date.now()
    let inFlight = false
    startWatch = setInterval(async () => {
      if (inFlight) return
      inFlight = true
      try {
        await fetchAutomations(true)
      } finally {
        inFlight = false
      }
      const now = selectedAutomation.value
      if (!now || now.is_running || now.queued || lastRunId(now) !== baseline || Date.now() - began >= START_WAIT_MAX_MS) stopWatchingStart()
    }, START_POLL_MS)
  }

  async function handleTrigger() {
    if (!selectedAutomation.value) return
    const baseline = lastRunId(selectedAutomation.value)
    let outcome: TriggerResponse | void = undefined
    triggering.value = true
    lastTriggerResult.value = `Running ${selectedAutomation.value.name}...`
    try {
      outcome = await triggerAutomation(
        selectedAutomation.value.workspace,
        selectedAutomation.value.name,
      )
      lastTriggerResult.value = `Started ${selectedAutomation.value.name}`
    } catch {
      lastTriggerResult.value = `Failed to trigger ${selectedAutomation.value.name}`
    } finally {
      triggering.value = false
      await fetchAutomations()
      await refreshHistory()
    }
    watchStart(outcome, baseline)
  }

  async function handleReplayRecording(auto: Automation, recording: RecordingMeta) {
    selectedAutomationId.value = auto.id
    const baseline = lastRunId(auto)
    let outcome: TriggerResponse | void = undefined
    triggering.value = true
    lastTriggerResult.value = `Replaying recording for ${auto.name}...`
    try {
      outcome = await triggerAutomation(auto.workspace, auto.name, recording.id)
      lastTriggerResult.value = `Replayed ${auto.name} from recording`
    } catch {
      lastTriggerResult.value = `Failed to replay ${auto.name}`
    } finally {
      triggering.value = false
      await fetchAutomations()
      await refreshHistory()
    }
    watchStart(outcome, baseline)
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
    stopWatchingStart()
    selectedAutomationId.value = null
    selectedRun.value = null
    lastTriggerResult.value = null
  }

  return {
    selectedAutomationId,
    selectedRun,
    triggering,
    starting,
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
