import { ref, onMounted, onUnmounted } from 'vue'
import { AdminApiService } from '../services/adminService'
import type { ProcessInfo } from '../types/admin'

const processes = ref<ProcessInfo[]>([])
let pollInterval: ReturnType<typeof setInterval> | null = null
let mountCount = 0

const refresh = async () => {
  try {
    const res = await AdminApiService.fetchProcesses()
    processes.value = res.processes
  } catch {
    // Silently retry on next poll
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

  return { processes, refresh }
}
