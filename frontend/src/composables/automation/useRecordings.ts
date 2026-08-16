import { ref } from 'vue'
import type { RecordingMeta, RecordingStatus } from '../../types/dispatcher'
import { DispatcherService } from '../../services/automation/dispatcherService'

const recordings = ref<RecordingMeta[]>([])
const status = ref<RecordingStatus>({ enabled: false, dir: '' })
const loading = ref(false)
const error = ref<string | null>(null)

export function useRecordings() {
  async function fetchStatus() {
    try {
      status.value = await DispatcherService.getRecordingStatus()
    } catch {
      status.value = { enabled: false, dir: '' }
    }
  }

  async function fetchRecordings(automation?: string) {
    loading.value = true
    error.value = null
    try {
      recordings.value = await DispatcherService.listRecordings(automation)
    } catch (e) {
      error.value = (e as Error).message
      recordings.value = []
    } finally {
      loading.value = false
    }
  }

  async function deleteRecording(id: string) {
    await DispatcherService.deleteRecording(id)
    recordings.value = recordings.value.filter(r => r.id !== id)
  }

  // Clear/set the recording_ref on an automation (which mode a replay uses).
  // Multi-step (getConfig -> find -> update), so it lives here rather than in a
  // presentation component.
  async function setAutomationRecordingRef(workspace: string, automation: string, recordingRef: string) {
    await DispatcherService.setAutomationRecordingRef(workspace, automation, recordingRef)
  }

  async function clearAutomationRecordingRef(workspace: string, automation: string) {
    await DispatcherService.clearAutomationRecordingRef(workspace, automation)
  }

  return {
    recordings,
    status,
    loading,
    error,
    fetchStatus,
    fetchRecordings,
    deleteRecording,
    setAutomationRecordingRef,
    clearAutomationRecordingRef,
  }
}
