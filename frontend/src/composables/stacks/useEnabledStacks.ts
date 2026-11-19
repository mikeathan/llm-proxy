import { computed, ref } from 'vue'
import { StacksService } from '../../services/stacks/stacksService'
import type { StackId } from '../../types/stacks'

// One answer for the whole app: the navigation and the dashboard both read it, so a stack switched on or off shows or hides everywhere at once.
const on = ref<ReadonlySet<StackId> | null>(null)

/**
 * Which stacks have any module switched on, from the list of stacks. Until the list is read (or when it cannot be) the answer is
 * `null`: "unknown", which callers treat as "offer everything" so a failed read never hides something the owner has.
 */
export function useEnabledStacks() {
  async function load(): Promise<void> {
    try {
      on.value = new Set((await StacksService.list()).filter((s) => s.on > 0).map((s) => s.id))
    } catch {
      on.value = null
    }
  }

  return { on: computed(() => on.value), load }
}
