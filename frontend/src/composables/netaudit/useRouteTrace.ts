import { computed, ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { ROUTE_LOAD_FAILED, ROUTE_START_FAILED } from '../../constants/netaudit'
import type { RouteTrace } from '../../types/netaudit'

// The traces are kept by the server for a while and outlive a visit to the tab, so the state is shared by every use of the composable: a trace
// started before leaving the tab is still shown filling in when it is opened again.
const traces = ref<RouteTrace[]>([])
const supported = ref(true)
const loaded = ref(false)
const starting = ref(false)
const error = ref('')

/**
 * The route trace (Constitution I.14): the routers between this computer and one internet address the last traffic look saw, traced when the
 * operator presses for it (the server asks for the passphrase). `refresh` is what a poller calls while a trace runs; it does nothing otherwise.
 */
export function useRouteTrace() {
  const running = computed(() => traces.value.some((t) => t.state === 'running'))
  const traceOf = (address: string): RouteTrace | null => traces.value.find((t) => t.address === address) ?? null

  async function load(): Promise<void> {
    try {
      const state = await NetauditService.routeTraces()
      traces.value = state.traces ?? []
      supported.value = state.supported
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, ROUTE_LOAD_FAILED)
    }
    loaded.value = true
  }

  /** Starts a trace; true when the server took it. */
  async function start(address: string): Promise<boolean> {
    if (starting.value || running.value) return false
    starting.value = true
    error.value = ''
    try {
      const state = await NetauditService.startRouteTrace(address)
      traces.value = state.traces ?? []
      supported.value = state.supported
      return true
    } catch (err) {
      error.value = errorMessage(err, ROUTE_START_FAILED)
      return false
    } finally {
      starting.value = false
    }
  }

  /** One read while a trace runs; nothing when none does. */
  async function refresh(): Promise<void> {
    if (running.value) await load()
  }

  return { traces, supported, loaded, starting, running, error, traceOf, load, start, refresh }
}
