import { computed } from 'vue'
import { useModels } from '../models/useModels'
import { FOLLOW_DEFAULT_CHANNEL_LABEL, IN_APP_CHANNEL, IN_APP_CHANNEL_LABEL } from '../../constants/reminders'
import type { ChoiceOption } from '../../types/ui'

// Where one reminder can be sent: the default channel setting, the web UI alone,
// and each connector type that is switched on.
export function useReminderChannels() {
  const { state } = useModels()
  return computed<ChoiceOption[]>(() => {
    const connectors = Object.values(state.value?.config?.communication?.connectors ?? {})
    const types = [...new Set(connectors.filter((c) => c.enabled).map((c) => c.type))]
    return [
      { value: '', label: FOLLOW_DEFAULT_CHANNEL_LABEL },
      { value: IN_APP_CHANNEL, label: IN_APP_CHANNEL_LABEL },
      ...types.map((type) => ({ value: type, label: type })),
    ]
  })
}
