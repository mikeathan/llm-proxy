import { ref } from 'vue'
import type { RecordingMeta, RecordingStatus } from '../../types/dispatcher'
import { DispatcherService } from '../../services/dispatcherService'

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

  return {
    recordings,
    status,
    loading,
    error,
    fetchStatus,
    fetchRecordings,
    deleteRecording
  }
}
