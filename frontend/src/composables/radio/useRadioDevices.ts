import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_DEVICES_FAILED, RADIO_FORGET_DONE } from '../../constants/radio'
import type { RadioClass, RadioOwnerDevice } from '../../types/radio'

/** Radio · My devices: the owner's named devices, and the one action that forgets everyone else's. */
export function useRadioDevices() {
  const devices = ref<RadioOwnerDevice[] | null>(null)
  const error = ref('')
  const notice = ref('')
  const busy = ref(false)

  async function run(work: () => Promise<void>, done = ''): Promise<void> {
    busy.value = true
    notice.value = ''
    try {
      await work()
      devices.value = await RadioService.ownerDevices()
      error.value = ''
      notice.value = done
    } catch (err) {
      error.value = errorMessage(err, RADIO_DEVICES_FAILED)
    } finally {
      busy.value = false
    }
  }

  return {
    devices, error, notice, busy,
    load: () => run(async () => {}),
    add: (address: string, label: string, cls?: RadioClass) => run(async () => { await (cls ? RadioService.addOwnerDevice(address, label, cls) : RadioService.addOwnerDevice(address, label)) }),
    remove: (id: string) => run(async () => { await RadioService.removeOwnerDevice(id) }),
    forget: () => run(async () => { await RadioService.forgetNeighbours() }, RADIO_FORGET_DONE),
  }
}
