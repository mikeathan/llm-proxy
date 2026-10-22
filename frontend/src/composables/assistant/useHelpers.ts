import { ref } from 'vue'
import { HelpersService } from '../../services/helpers/helpersService'
import type { HelpersStatus } from '../../types/helpers'

// Whether helpers can run, and on which model, read from the server rather than worked out from settings. Nothing is
// read until refresh(): the chat calls it whenever it opens, so a Helper model changed in Settings shows at once.
const status = ref<HelpersStatus>({ available: false, model: '' })
let inFlight: Promise<void> | null = null

function read(): Promise<void> {
  inFlight ??= HelpersService.status()
    .then((next) => {
      status.value = next
    })
    .catch(() => {
      // Unreachable or old server: helpers stay off, and the next refresh tries again.
      status.value = { available: false, model: '' }
    })
    .finally(() => {
      inFlight = null
    })
  return inFlight
}

export function useHelpers() {
  return { status, refresh: read }
}
