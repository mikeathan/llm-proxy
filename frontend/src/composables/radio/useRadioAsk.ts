import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_ASK_FAILED } from '../../constants/radioIdentity'
import type { RadioAsk, RadioClass } from '../../types/radio'

/** Radio · the few devices your answer would help most with, and the answers: "mine" names it yours, "not mine" says it is a neighbour's. */
export function useRadioAsk() {
  const view = ref<RadioAsk | null>(null)
  const error = ref('')
  const busy = ref('')
  async function load(): Promise<void> {
    try {
      view.value = await RadioService.ask()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_ASK_FAILED)
    }
  }
  async function answer(id: string, run: () => Promise<unknown>): Promise<boolean> {
    busy.value = id
    try {
      await run()
      error.value = ''
      await load()
      return true
    } catch (err) {
      error.value = errorMessage(err, RADIO_ASK_FAILED)
      return false
    } finally {
      busy.value = ''
    }
  }
  const mine = (id: string, label: string): Promise<boolean> => answer(id, () => RadioService.markMine(id, label))
  const setClass = (id: string, cls: RadioClass | 'not_mine'): Promise<boolean> => answer(id, () => RadioService.setClass(id, cls))
  return { view, error, busy, load, mine, setClass }
}
