import { computed } from 'vue'
import { useModels } from '../models/useModels'
import { TOPIC_FEED_ONLY } from '../../constants/newsroom'
import { enabledConnectorTypes } from '../../utils/reminders/channels'
import type { ChoiceOption } from '../../types/ui'

// Where a topic's message can go: nowhere (the feed only, which every topic has), or a connector type that is switched on.
export function useNewsroomChannels() {
  const { state } = useModels()
  return computed<ChoiceOption[]>(() => [
    { value: '', label: TOPIC_FEED_ONLY },
    ...enabledConnectorTypes(state.value?.config?.communication?.connectors).map((type) => ({ value: type, label: type })),
  ])
}
