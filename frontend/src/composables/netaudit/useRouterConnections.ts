import { computed, inject, ref, type InjectionKey } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { ROUTER_CONN_FAILED, ROUTER_CONN_LOAD_FAILED } from '../../constants/netaudit'
import type { RouterConnState, RouterSetupStep } from '../../types/netaudit'

type Busy = '' | 'target' | 'key' | 'fingerprint' | 'pin' | 'read'

/**
 * The router's connection table (Constitution I.15): where and as whom to read it, the key, the router's own key to trust, and the read. Every
 * change and the read ask for the passphrase on the server. The key is sent once and never comes back. The last read is kept by the server in
 * memory, so it is there again after a visit to another tab.
 */
export function useRouterConnections() {
  const state = ref<RouterConnState | null>(null)
  const loaded = ref(false)
  const busy = ref<Busy>('')
  const error = ref('')
  // The fingerprint the router showed and the operator has not yet trusted.
  const candidate = ref('')

  const step = computed<RouterSetupStep>(() => {
    const s = state.value
    if (!s || !s.address || !s.user) return 'target'
    if (!s.key_saved) return 'key'
    if (!s.fingerprint) return 'trust'
    return 'ready'
  })

  async function run(what: Exclude<Busy, ''>, call: () => Promise<RouterConnState | void>): Promise<boolean> {
    if (busy.value) return false
    busy.value = what
    error.value = ''
    try {
      const next = await call()
      if (next) state.value = next
      return true
    } catch (err) {
      error.value = errorMessage(err, ROUTER_CONN_FAILED)
      return false
    } finally {
      busy.value = ''
    }
  }

  async function load(): Promise<void> {
    try {
      state.value = await NetauditService.routerConn()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, ROUTER_CONN_LOAD_FAILED)
    }
    loaded.value = true
  }

  const saveTarget = (address: string, user: string) => run('target', () => NetauditService.setRouterTarget(address.trim(), user.trim()))
  const saveKey = (key: string) => run('key', () => NetauditService.setRouterKey(key))
  const removeKey = () => run('key', async () => { candidate.value = ''; return NetauditService.deleteRouterKey() })
  const check = () => run('fingerprint', async () => { candidate.value = (await NetauditService.routerFingerprint()).fingerprint })
  const trust = () => run('pin', async () => {
    const next = await NetauditService.pinRouter(candidate.value)
    candidate.value = ''
    return next
  })
  const read = () => run('read', () => NetauditService.readRouter())

  return { state, loaded, busy, error, candidate, step, load, saveTarget, saveKey, removeKey, check, trust, read }
}

/** The Traffic tab provides one instance, so the setup card that reads the router and the device list that shows the reading share one state. */
export const ROUTER_CONN_INJECTION: InjectionKey<ReturnType<typeof useRouterConnections>> = Symbol('routerConnections')

/** The tab's instance; a part used on its own (a test, a later page) gets one of its own. */
export function useSharedRouterConnections(): ReturnType<typeof useRouterConnections> {
  return inject(ROUTER_CONN_INJECTION, null) ?? useRouterConnections()
}
