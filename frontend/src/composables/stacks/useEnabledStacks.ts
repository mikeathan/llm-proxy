import { computed, ref } from 'vue'
import { StacksService } from '../../services/stacks/stacksService'
import type { StackId } from '../../types/stacks'

/**
 * Which stacks have any module switched on, from the list of stacks. Until the list is read (or when it cannot be) the answer is
 * `null`: "unknown", which callers treat as "offer everything" so a failed read never hides something the owner has.
 */
export function useEnabledStacks() {
  const on = ref<ReadonlySet<StackId> | null>(null)

  async function load(): Promise<void> {
    try {
      on.value = new Set((await StacksService.list()).filter((s) => s.on > 0).map((s) => s.id))
    } catch {
      on.value = null
    }
  }

  return { on: computed(() => on.value), load }
}
