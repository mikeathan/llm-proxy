import { onScopeDispose, ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { NEARBY_LOAD_FAILED, NEARBY_LOOK_FAILED } from '../../constants/netaudit'
import type { NearbyState } from '../../types/netaudit'

/** How often the state is read while a look is running. */
const POLL_MS = 1000

/**
 * Nearby as its card and its settings switch show it. A change answers with the whole new state, which replaces the old; while a
 * look runs the state is read once a second until the server says it has ended. The poll stops with its owner.
 */
export function useNearby() {
  const state = ref<NearbyState | null>(null)
  const error = ref('')
  const busy = ref(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  // Set once the owner is gone: an answer that arrives afterwards must not start the poll again.
  let disposed = false

  const stop = () => {
    if (timer) clearTimeout(timer)
    timer = undefined
  }
  onScopeDispose(() => {
    disposed = true
    stop()
  })

  async function load(): Promise<void> {
    try {
      state.value = await NetauditService.nearby()
      error.value = ''
      if (state.value.running) poll()
    } catch (err) {
      error.value = errorMessage(err, NEARBY_LOAD_FAILED)
    }
  }

  function poll() {
    stop()
    if (!disposed) timer = setTimeout(load, POLL_MS)
  }

  async function run(call: () => Promise<NearbyState>, failed: string): Promise<boolean> {
    busy.value = true
    try {
      state.value = await call()
      error.value = ''
      if (state.value.running) poll()
      return true
    } catch (err) {
      error.value = errorMessage(err, failed)
      return false
    } finally {
      busy.value = false
    }
  }

  return {
    state, error, busy, load,
    check: () => run(() => NetauditService.nearbyLook(), NEARBY_LOOK_FAILED),
  }
}
