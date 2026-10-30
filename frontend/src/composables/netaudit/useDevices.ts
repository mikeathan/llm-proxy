import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import {
  DEVICES_ACTION_FAILED,
  DEVICES_LOAD_FAILED,
  IPV6_LOAD_FAILED,
  IPV6_SAVE_FAILED,
  PRESENCE_LOAD_FAILED,
  PRESENCE_SAVE_FAILED,
} from '../../constants/netaudit'
import type { DeviceAction, DeviceInventory, IPv6DiscoveryState, PresenceState } from '../../types/netaudit'

/**
 * The devices seen on the network and the new-device check, as the settings card shows them. A change answers with the
 * whole new list, which replaces the old one, so the screen never edits what it holds.
 */
export function useDevices() {
  const inventory = ref<DeviceInventory | null>(null)
  const presence = ref<PresenceState | null>(null)
  const ipv6 = ref<IPv6DiscoveryState | null>(null)
  const loadError = ref('')
  const actionError = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    loadError.value = ''
    const [devices, state, discovery] = await Promise.allSettled([NetauditService.devices(), NetauditService.presence(), NetauditService.ipv6Discovery()])
    if (devices.status === 'fulfilled') inventory.value = devices.value
    else loadError.value = errorMessage(devices.reason, DEVICES_LOAD_FAILED)
    if (state.status === 'fulfilled') presence.value = state.value
    else if (!loadError.value) loadError.value = errorMessage(state.reason, PRESENCE_LOAD_FAILED)
    if (discovery.status === 'fulfilled') ipv6.value = discovery.value
    else if (!loadError.value) loadError.value = errorMessage(discovery.reason, IPV6_LOAD_FAILED)
  }

  async function act(action: DeviceAction, call: () => Promise<DeviceInventory>): Promise<boolean> {
    busy.value = true
    try {
      inventory.value = await call()
      actionError.value = ''
      return true
    } catch (err) {
      actionError.value = errorMessage(err, DEVICES_ACTION_FAILED[action])
      return false
    } finally {
      busy.value = false
    }
  }

  async function setPresence(enabled: boolean, intervalMinutes: number): Promise<void> {
    busy.value = true
    try {
      presence.value = await NetauditService.setPresence({ enabled, interval_minutes: intervalMinutes })
      actionError.value = ''
    } catch (err) {
      actionError.value = errorMessage(err, PRESENCE_SAVE_FAILED)
    } finally {
      busy.value = false
    }
  }

  async function setIPv6(enabled: boolean): Promise<void> {
    busy.value = true
    try {
      ipv6.value = await NetauditService.setIpv6Discovery(enabled)
      actionError.value = ''
    } catch (err) {
      actionError.value = errorMessage(err, IPV6_SAVE_FAILED)
    } finally {
      busy.value = false
    }
  }

  return {
    inventory,
    presence,
    ipv6,
    loadError,
    actionError,
    busy,
    load,
    setPresence,
    setIPv6,
    name: (mac: string, name: string) => act('name', () => NetauditService.nameDevice(mac, name)),
    approve: (mac: string) => act('approve', () => NetauditService.approveDevice(mac)),
    forget: (mac: string) => act('forget', () => NetauditService.forgetDevice(mac)),
    confirmGateway: () => act('confirm-gateway', () => NetauditService.confirmGateway()),
  }
}
