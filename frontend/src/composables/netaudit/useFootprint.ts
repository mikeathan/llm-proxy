import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { FOOTPRINT_CHECK_FAILED, FOOTPRINT_LOAD_FAILED, FOOTPRINT_SAVE_FAILED } from '../../constants/netaudit'
import type { FootprintCommand, FootprintState } from '../../types/netaudit'

/** The footprint card's state. Every change answers with the whole new page, which replaces the old; one watch is checked at a time. */
export function useFootprint() {
  const state = ref<FootprintState | null>(null)
  const error = ref('')
  const busy = ref(false)
  const checking = ref('')

  async function load(): Promise<void> {
    try {
      state.value = await NetauditService.footprint()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, FOOTPRINT_LOAD_FAILED)
    }
  }

  async function change(run: () => Promise<FootprintState>): Promise<boolean> {
    busy.value = true
    try {
      state.value = await run()
      error.value = ''
      return true
    } catch (err) {
      error.value = errorMessage(err, FOOTPRINT_SAVE_FAILED)
      return false
    } finally {
      busy.value = false
    }
  }

  async function check(label: string): Promise<void> {
    if (checking.value) return
    checking.value = label
    try {
      state.value = await NetauditService.checkFootprint(label)
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, FOOTPRINT_CHECK_FAILED)
    } finally {
      checking.value = ''
    }
  }

  return {
    state, error, busy, checking, load, check,
    apply: (command: FootprintCommand) => change(() => NetauditService.changeFootprint(command)),
    saveKey: (key: string) => change(() => NetauditService.setFootprintKey(key.trim())),
    removeKey: () => change(() => NetauditService.clearFootprintKey()),
  }
}
