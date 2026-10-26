import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import type { Readiness } from '../../types/netaudit'

// What is missing before scans can run. One shared answer: the settings tab and the automation page show the same
// banner, and a fix made on one clears it on the other after a refresh.
const readiness = ref<Readiness | null>(null)
const error = ref('')
const loading = ref(false)

async function refresh(): Promise<void> {
  loading.value = true
  try {
    readiness.value = await NetauditService.readiness()
    error.value = ''
  } catch (err) {
    error.value = errorMessage(err, 'The scan requirements could not be read.')
  } finally {
    loading.value = false
  }
}

export function useNetauditReadiness() {
  return { readiness, error, loading, refresh }
}
