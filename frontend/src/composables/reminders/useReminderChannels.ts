import { computed } from 'vue'
import { useModels } from '../models/useModels'
import { AUTOMATIC_CHANNEL_LABEL, IN_APP_CHANNEL, IN_APP_CHANNEL_LABEL } from '../../constants/reminders'
import type { ChoiceOption } from '../../types/ui'

// Where a reminder can be sent: automatic (the default rule), the web UI alone,
// and each connector type that is switched on.
export function useReminderChannels() {
  const { state } = useModels()
  return computed<ChoiceOption[]>(() => {
    const connectors = Object.values(state.value?.config?.communication?.connectors ?? {})
    const types = [...new Set(connectors.filter((c) => c.enabled).map((c) => c.type))]
    return [
      { value: '', label: AUTOMATIC_CHANNEL_LABEL },
      { value: IN_APP_CHANNEL, label: IN_APP_CHANNEL_LABEL },
      ...types.map((type) => ({ value: type, label: type })),
    ]
  })
}
