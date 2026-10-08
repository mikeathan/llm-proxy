import { computed, type Ref } from 'vue'
import { useModels } from '../models/useModels'
import type { ChoiceOption } from '../../types/ui'

// The communication connectors a result can be sent to. A connector already
// selected stays listed even if it was since removed, so editing never
// silently drops it.
export function useConnectorOptions(selected: Readonly<Ref<string>>) {
  const { state } = useModels()
  const connectorOptions = computed<ChoiceOption[]>(() => {
    const configured = state.value?.config?.communication?.connectors ?? {}
    const options = Object.entries(configured).map(([name, c]) => ({
      value: name,
      label: c.enabled ? name : `${name} (disabled)`,
    }))
    if (selected.value && !(selected.value in configured)) {
      options.push({ value: selected.value, label: `${selected.value} (not configured)` })
    }
    return options
  })
  // True only once the admin state has loaded and it lists no connector, so the
  // "add one" note never flashes while the state is still loading.
  const noConnectors = computed(
    () => !!state.value && Object.keys(state.value.config?.communication?.connectors ?? {}).length === 0,
  )
  return { connectorOptions, noConnectors }
}
