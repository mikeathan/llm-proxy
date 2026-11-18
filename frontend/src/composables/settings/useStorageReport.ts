import { ref } from 'vue'
import { AdminApiService } from '../../services/admin/adminService'
import { errorMessage } from '../../utils/errors'
import { STORAGE_FAILED } from '../../constants/storage'
import type { StorageReport } from '../../types/storage'

/** Settings · What is stored: the data folder's report, read when the card opens and again on Refresh. */
export function useStorageReport() {
  const report = ref<StorageReport | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    busy.value = true
    try {
      report.value = await AdminApiService.storage()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, STORAGE_FAILED)
    } finally {
      busy.value = false
    }
  }

  return { report, error, busy, load }
}
