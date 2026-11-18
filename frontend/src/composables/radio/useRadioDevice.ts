import { ref, type Ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_ASK_FAILED, RADIO_DEVICE_GONE } from '../../constants/radioIdentity'
import { HttpError } from '../../services/httpClient'
import type { RadioClass, RadioDeviceKind, RadioDeviceView } from '../../types/radio'

/** Radio · one device's page: the device in full, and the owner's answers about it (whose, what it is, the distance calibration). */
export function useRadioDevice(id: Ref<string>) {
  const view = ref<RadioDeviceView | null>(null)
  const error = ref('')
  const gone = ref(false)
  const busy = ref(false)
  const saved = ref(false)

  async function load(): Promise<void> {
    const want = id.value
    try {
      const v = await RadioService.device(want)
      if (want !== id.value) return
      view.value = v
      error.value = ''
      gone.value = false
    } catch (err) {
      if (want !== id.value) return
      gone.value = err instanceof HttpError && err.status === 404
      error.value = gone.value ? RADIO_DEVICE_GONE : errorMessage(err, RADIO_DEVICE_GONE)
    }
  }

  async function act(run: () => Promise<unknown>): Promise<boolean> {
    busy.value = true
    saved.value = false
    try {
      await run()
      saved.value = true
      error.value = ''
      await load()
      return true
    } catch (err) {
      error.value = errorMessage(err, RADIO_ASK_FAILED)
      return false
    } finally {
      busy.value = false
    }
  }

  return {
    view, error, gone, busy, saved, load,
    markMine: (label: string) => act(() => RadioService.markMine(id.value, label)),
    setClass: (cls: RadioClass | '' | 'not_mine') => act(() => RadioService.setClass(id.value, cls)),
    setKind: (kind: RadioDeviceKind) => act(() => RadioService.setKind(id.value, kind)),
    calibrate: (metres: number) => act(() => RadioService.calibrate(id.value, metres)),
  }
}
