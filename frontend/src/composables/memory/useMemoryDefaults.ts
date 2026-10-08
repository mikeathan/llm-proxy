import { computed } from 'vue'
import { useModels } from '../models/useModels'
import { SHIPPED_MEMORY_DEFAULTS } from '../../constants/memory'

// The global hot-memory defaults as the backend last reported them: what an
// automation or a workspace's assistant gets when it does not override them.
export function useMemoryDefaults() {
  const { state } = useModels()
  const defaults = computed(() => state.value?.config?.memory ?? SHIPPED_MEMORY_DEFAULTS)
  return {
    assistantDefaultOn: computed(() => defaults.value.assistant_hot),
    automationDefaultOn: computed(() => defaults.value.automation_hot),
  }
}
