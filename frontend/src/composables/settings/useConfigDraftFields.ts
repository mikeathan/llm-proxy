import { computed } from 'vue'
import type { GlobalConfig } from '../../types/admin'

/**
 * Fields of the shared configuration draft (useConfig) as v-model targets. The page that owns the draft passes it in as a prop and takes
 * each change back through `update` as a whole new copy, so a form component never mutates its prop. Used by Settings · General and
 * the Models page's Engine and Routing tabs.
 */
export function useConfigDraftFields(draft: () => GlobalConfig, update: (next: GlobalConfig) => void) {
  const replaceConfig = (patch: Partial<GlobalConfig>) => update({ ...draft(), ...patch })

  /** A top-level config field as a v-model target that writes a copy. */
  const configField = <K extends keyof GlobalConfig>(key: K) =>
    computed<GlobalConfig[K]>({
      get: () => draft()[key],
      set: (value) => replaceConfig({ [key]: value } as Partial<GlobalConfig>),
    })

  return { replaceConfig, configField }
}
