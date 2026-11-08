import { computed, ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { HELPER_UDP_PORTS, PROBE_CLEAR_FAILED, PROBE_FAILED } from '../../constants/netaudit'
import type { HelperRunResult, HelperStatus, OsGuess, ProbeAction, UdpPort } from '../../types/netaudit'

// The actions the page can run on one device, in the order the buttons show them.
const PROBE_ACTIONS: readonly ProbeAction[] = ['scan_udp', 'os_detect']

/**
 * A UDP scan or an OS detection of one device through the privileged helper. The helper's state is read once; a button is offered for an
 * action only when the helper is ready and the action is both switched on and in the helper's own list. Each run asks for the passphrase
 * (the server's step-up; the page asks and sends the request again), so nothing here keeps a session open. The server keeps the result on
 * the device; the newest one is also held here so it shows at once, before the list is read again.
 */
export function useDeviceProbe(ip: () => string | undefined, mac: () => string | undefined = () => undefined) {
  const status = ref<HelperStatus | null>(null)
  const running = ref<ProbeAction | null>(null)
  const freshUdp = ref<UdpPort[] | null>(null)
  const freshOs = ref<OsGuess[] | null>(null)
  const error = ref('')

  // A failed read of the state offers nothing: the Network settings card is where the helper's trouble is shown.
  async function load(): Promise<void> {
    try {
      status.value = await NetauditService.helper()
    } catch {
      status.value = null
    }
  }

  const actions = computed<ProbeAction[]>(() => {
    const st = status.value
    if (!ip() || !st?.ready) return []
    return PROBE_ACTIONS.filter((id) => st.actions.some((a) => a.id === id && a.on && a.installed !== false))
  })

  /** Runs one action; true when the server answered with a result it could read. */
  async function run(action: ProbeAction): Promise<boolean> {
    const target = ip()
    if (!target || running.value) return false
    running.value = action
    error.value = ''
    try {
      const answer: HelperRunResult = action === 'scan_udp'
        ? await NetauditService.scanUdp(target, [...HELPER_UDP_PORTS])
        : await NetauditService.detectOs(target)
      if (!answer.parsed) {
        error.value = answer.parse_error || PROBE_FAILED
        return false
      }
      if (action === 'scan_udp') freshUdp.value = answer.parsed.udp_ports
      else freshOs.value = answer.parsed.os
      return true
    } catch (err) {
      error.value = errorMessage(err, PROBE_FAILED)
      return false
    } finally {
      running.value = null
    }
  }

  /** Drops the kept UDP scan or OS detection of this device; true when the server did. A new scan replaces it as well. */
  async function clear(kind: 'udp' | 'os'): Promise<boolean> {
    const id = mac()
    if (!id) return false
    error.value = ''
    try {
      await NetauditService.clearProbe(id, kind)
      if (kind === 'udp') freshUdp.value = null
      else freshOs.value = null
      return true
    } catch (err) {
      error.value = errorMessage(err, PROBE_CLEAR_FAILED)
      return false
    }
  }

  return { actions, running, freshUdp, freshOs, error, load, run, clear }
}
