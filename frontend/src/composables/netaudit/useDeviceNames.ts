import { computed, ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { namesByAddress } from '../../utils/netaudit/devices'
import type { NetworkDevice } from '../../types/netaudit'

/**
 * What the person calls each device, by address, so a finding that names an address can say "Pixel 8 · 192.168.50.10". The names only
 * add to a line, so a failure to read them leaves none and the screen shows addresses, as it did before.
 */
export function useDeviceNames() {
  const devices = ref<NetworkDevice[]>([])
  async function load(): Promise<void> {
    try {
      const inventory = await NetauditService.devices()
      devices.value = Array.isArray(inventory?.devices) ? inventory.devices : []
    } catch {
      devices.value = []
    }
  }
  /** The device that holds this IPv4 address, or none (an address of the internet, or of a device the inventory has not seen). */
  function deviceAt(address: string): NetworkDevice | undefined {
    return devices.value.find((d) => d.ip === address)
  }
  /** The device with this hardware address, or none. */
  function deviceByMac(mac: string): NetworkDevice | undefined {
    return devices.value.find((d) => d.mac.toLowerCase() === mac.toLowerCase())
  }
  return { devices, names: computed(() => namesByAddress(devices.value)), deviceAt, deviceByMac, load }
}
