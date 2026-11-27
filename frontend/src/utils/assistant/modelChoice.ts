import type { ChoiceOption } from '../../types/ui'
import type { Model } from '../../types/model'
import type { ProviderType } from '../../types/admin'
import { isLocalModel, modelChoiceLabel, modelProviderName } from '../../domain/models'

// The assistant's model picker: "" means no choice, so the server runs its primary model.
export const DEFAULT_MODEL_VALUE = ''
const DEFAULT_LABEL = 'Default'

type ProviderLabel = (p: ProviderType) => string

/**
 * The picker's options: the default first (named after the primary model and its provider), then every registry model named plainly and
 * grouped under where it runs (local first, then the providers by name), so a long name never carries its provider in the same line.
 */
export function chatModelOptions(models: readonly Model[], primary: string, providerLabel: ProviderLabel): ChoiceOption[] {
  const primaryModel = models.find((m) => m.name === primary)
  const defaultLabel = primaryModel ? `${DEFAULT_LABEL} (${modelChoiceLabel(primaryModel, providerLabel)})` : primary ? `${DEFAULT_LABEL} (${primary})` : DEFAULT_LABEL
  return [{ value: DEFAULT_MODEL_VALUE, label: defaultLabel }, ...orderForGroups(models, providerLabel).map((m) => ({ value: m.name, label: m.name, group: modelProviderName(m, providerLabel) }))]
}

function orderForGroups(models: readonly Model[], providerLabel: ProviderLabel): Model[] {
  const rank = (m: Model) => (isLocalModel(m) ? '' : modelProviderName(m, providerLabel).toLowerCase())
  return models.map((m, i) => ({ m, i })).sort((a, b) => (rank(a.m) < rank(b.m) ? -1 : rank(a.m) > rank(b.m) ? 1 : a.i - b.i)).map(({ m }) => m)
}

/** The stored choice if the registry still has that model; otherwise the default. */
export function chosenModel(stored: string, names: string[]): string {
  return stored && names.includes(stored) ? stored : DEFAULT_MODEL_VALUE
}
