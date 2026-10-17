import { computed } from 'vue'
import { useModels } from '../models/useModels'
import { usePersistedState } from '../ui/usePersistedState'
import { chatModelOptions, chosenModel, DEFAULT_MODEL_VALUE } from '../../utils/assistant/modelChoice'

const STORAGE_KEY = 'assistant-model'
const STORAGE_VERSION = 1

// The model the operator picked for the assistant, kept in this browser. One choice
// for every chat on the page (the Assistant page and the ask panel), so they agree.
let stored: ReturnType<typeof usePersistedState<string>> | undefined

export function useAssistantModel() {
  stored ??= usePersistedState<string>(STORAGE_KEY, {
    version: STORAGE_VERSION,
    parse: (data) => (typeof data === 'string' ? data : null),
    fallback: DEFAULT_MODEL_VALUE,
  })
  const { models, state } = useModels()

  const names = computed(() => models.value.map((m) => m.name))
  const options = computed(() => chatModelOptions(names.value, state.value?.config?.primary_model ?? ''))
  // A removed model must not be sent: the server would refuse the message.
  const model = computed({
    get: () => chosenModel(stored!.value, names.value),
    set: (value: string) => { stored!.value = value },
  })

  return { model, options }
}
