import { ref, onMounted, onUnmounted } from 'vue'
import { AdminApiService } from '../../services/admin/adminService'
import { errorMessage } from '../../utils/errors'
import type { ProcessInfo } from '../../types/admin'

const processes = ref<ProcessInfo[]>([])
// Whether a list has arrived yet, and why the last poll failed (cleared by the
// next success) — so an empty table is never shown before the first answer.
const loaded = ref(false)
const error = ref('')
let pollInterval: ReturnType<typeof setInterval> | null = null
let mountCount = 0

const refresh = async () => {
  try {
    const res = await AdminApiService.fetchProcesses()
    processes.value = res.processes
    error.value = ''
  } catch (e) {
    // Shown with the table; the next poll retries.
    error.value = errorMessage(e)
  } finally {
    loaded.value = true
  }
}

export function useProcesses() {
  onMounted(() => {
    mountCount++
    if (mountCount === 1) {
      refresh()
      pollInterval = setInterval(refresh, 10000)
    }
  })

  onUnmounted(() => {
    mountCount--
    if (mountCount === 0 && pollInterval) {
      clearInterval(pollInterval)
      pollInterval = null
    }
  })

  return { processes, loaded, error, refresh }
}
