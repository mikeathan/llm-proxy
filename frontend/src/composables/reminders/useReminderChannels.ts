import { computed } from 'vue'
import { useModels } from '../models/useModels'
import { IN_APP_CHANNEL, IN_APP_CHANNEL_LABEL } from '../../constants/reminders'
import { defaultConnectorTypes, deliveryLabel, enabledConnectorTypes } from '../../utils/reminders/channels'
import type { ChoiceOption } from '../../types/ui'

// Where one reminder can be sent: the default (named with what it means right now), the web UI alone,
// and each connector type that is switched on.
export function useReminderChannels() {
  const { state } = useModels()
  return computed<ChoiceOption[]>(() => {
    const config = state.value?.config
    const types = enabledConnectorTypes(config?.communication?.connectors)
    const defaults = defaultConnectorTypes(types, config?.reminders?.skip_connectors)
    return [
      { value: '', label: `Default: ${deliveryLabel(defaults)}` },
      { value: IN_APP_CHANNEL, label: IN_APP_CHANNEL_LABEL },
      ...types.map((type) => ({ value: type, label: type })),
    ]
  })
}
