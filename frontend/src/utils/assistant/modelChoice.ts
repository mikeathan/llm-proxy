import type { ChoiceOption } from '../../types/ui'

// The assistant's model picker: "" means no choice, so the server runs its primary model.
export const DEFAULT_MODEL_VALUE = ''
const DEFAULT_LABEL = 'Default'

/** The picker's options: the default first (named after the primary model), then every registry model. */
export function chatModelOptions(names: string[], primary: string): ChoiceOption[] {
  const defaultLabel = primary ? `${DEFAULT_LABEL} (${primary})` : DEFAULT_LABEL
  return [{ value: DEFAULT_MODEL_VALUE, label: defaultLabel }, ...names.map((name) => ({ value: name, label: name }))]
}

/** The stored choice if the registry still has that model; otherwise the default. */
export function chosenModel(stored: string, names: string[]): string {
  return stored && names.includes(stored) ? stored : DEFAULT_MODEL_VALUE
}
