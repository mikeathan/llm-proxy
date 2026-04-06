/**
 * Model domain utilities — pure, stateless functions.
 *
 * Follows the Single Responsibility Principle:
 * all model-form and provider-mapping logic lives here,
 * keeping components free of business-logic noise.
 */
import type { ProviderType } from '../types/admin'
import type { NewModelForm, Model } from '../types/model'


/** Maps a dashboard tab to the default provider for a new model form. */
export function tabToDefaultProvider(tab: 'local' | 'cloud'): ProviderType {
  return tab === 'local' ? 'local' : 'gemini'
}

/** Returns true if the given provider is the local llama.cpp engine. */
export function isLocalProvider(provider: string): boolean {
  return provider === 'local'
}

/** Creates a blank NewModelForm for the given provider. */
export function makeEmptyForm(provider: ProviderType): NewModelForm {
  return {
    name: '',
    provider,
    filename: '',
    port: 0,
    args: '',
    model_id: '',
    provider_config: { api_key_name: '' }
  }
}

/** Filters a model list to only those matching the current dashboard tab. */
export function filterModelsByTab(models: Model[], tab: 'local' | 'cloud'): Model[] {
  return models.filter((m) =>
    tab === 'local' ? isLocalProvider(m.provider) : !isLocalProvider(m.provider)
  )
}

/** Normalises a raw form's args string into a string[] ready for the API. */
export function normaliseFormArgs(args: string | undefined): string[] {
  return (args || '').split(' ').filter(Boolean)
}
