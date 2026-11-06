import { computed, ref } from 'vue'
import { StacksService } from '../../services/stacks/stacksService'
import type { StackModule, StackView } from '../../types/stacks'

// The Network stack, read once and shared by everything that wants to say which module does what: the coverage line on the overview,
// the "found by" line on a finding and the Help. It is read, never changed, here; the Modules tab owns the switches.
const stack = ref<StackView | null>(null)

export function useModuleSources() {
  const modules = computed<StackModule[]>(() => stack.value?.layers?.flatMap((l) => l.modules ?? []) ?? [])
  const byCheck = computed(() => {
    const out = new Map<string, StackModule>()
    for (const m of modules.value) for (const check of m.finds ?? []) out.set(check, m)
    return out
  })
  /** The module that produces a check's findings, or null when none does alone (or the stack has not been read). */
  const sourceOf = (check: string): StackModule | null => byCheck.value.get(check) ?? null

  async function load(): Promise<void> {
    try {
      stack.value = await StacksService.stack('network')
    } catch {
      // These lines are a convenience: without the stack the page simply shows no line, and the Modules tab says what went wrong.
    }
  }

  return { modules, stack, sourceOf, load }
}
