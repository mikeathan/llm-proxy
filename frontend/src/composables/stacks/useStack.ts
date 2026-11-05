import { ref } from 'vue'
import { StacksService } from '../../services/stacks/stacksService'
import { errorMessage } from '../../utils/errors'
import { STACK_LOAD_FAILED, STACK_SWITCH_FAILED } from '../../constants/stacks'
import type { StackId, StackView } from '../../types/stacks'

/**
 * One stack as its Modules tab shows it. A switch answers with the whole stack, which replaces the old one; a refusal leaves the
 * stack as the server last said and names the module it was about, so the row can say why next to itself.
 */
export function useStack(id: StackId) {
  const stack = ref<StackView | null>(null)
  const error = ref('')
  const errorFor = ref<{ module: string; message: string } | null>(null)
  const busyModule = ref<string | null>(null)
  const leftBlocked = ref<string[]>([])

  async function load(): Promise<void> {
    try {
      stack.value = await StacksService.stack(id)
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, STACK_LOAD_FAILED)
    }
  }

  async function set(module: string, enabled: boolean): Promise<void> {
    busyModule.value = module
    try {
      const { blocked, ...next } = await StacksService.setModule(id, module, enabled)
      stack.value = next
      leftBlocked.value = blocked
      errorFor.value = null
    } catch (err) {
      errorFor.value = { module, message: errorMessage(err, STACK_SWITCH_FAILED) }
    } finally {
      busyModule.value = null
    }
  }

  return { stack, error, errorFor, busyModule, leftBlocked, load, set }
}
