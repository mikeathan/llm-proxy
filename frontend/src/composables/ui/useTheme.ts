import { computed, ref, watch } from 'vue'
import type {
  AppliedTheme,
  CustomTheme,
  PresetId,
  SaveThemeOutcome,
  ThemeEnvironment,
  ThemeSelection,
  ThemeTokens,
} from '../../types/theme'
import { applyTheme } from '../../theme/apply'
import { channelsToHex, parseColour } from '../../theme/colour'
import { DEFAULT_PRESET_ID, isPresetId, PRESETS, PRESET_IDS } from '../../theme/presets'
import { TOKEN_REGISTRY, isTokenName } from '../../theme/tokenRegistry'
import { validateCustomTheme } from '../../theme/validate'
import { browserStorage, usePersistedState } from './usePersistedState'

// The theme singleton (plan D2, D3, D19): current theme, presets, custom themes,
// persistence and the OS-scheme listener. `createThemeController` takes its
// environment so tests can inject storage / matchMedia / root; `useTheme()`
// returns the one app instance, started once from main.ts.

const LIGHT_QUERY = '(prefers-color-scheme: light)'
const SYSTEM_LIGHT_PRESET: PresetId = 'retro-paper'
const STORAGE_VERSION = 1
/** Read by public/theme-boot.js — keep the key in step with it. */
export const APPLIED_THEME_KEY = 'theme-applied'

function parseSelection(data: unknown): ThemeSelection | null {
  if (typeof data !== 'object' || data === null) return null
  const { kind, id } = data as Record<string, unknown>
  if (kind === 'preset' && isPresetId(id)) return { kind, id }
  if (kind === 'custom' && typeof id === 'string') return { kind, id }
  return null
}

function parseCustomThemes(data: unknown): CustomTheme[] | null {
  if (!Array.isArray(data)) return null
  return data.flatMap((item) => {
    const result = validateCustomTheme(item)
    return result.ok ? [result.theme] : []
  })
}

function exportable(overrides: ThemeTokens): ThemeTokens {
  const out: ThemeTokens = {}
  for (const [name, value] of Object.entries(overrides)) {
    if (!isTokenName(name)) continue
    const rgb = TOKEN_REGISTRY[name].kind === 'colour' ? parseColour(value) : null
    out[name] = rgb ? channelsToHex(rgb) : value
  }
  return out
}

export function createThemeController(env: ThemeEnvironment) {
  const { storage } = env
  const selection = usePersistedState<ThemeSelection | null>('theme-selection', {
    version: STORAGE_VERSION,
    parse: parseSelection,
    fallback: null,
    storage,
  })
  const customThemes = usePersistedState<CustomTheme[]>('custom-themes', {
    version: STORAGE_VERSION,
    parse: parseCustomThemes,
    fallback: [],
    storage,
  })
  const media = env.matchMedia?.(LIGHT_QUERY)
  const systemPrefersLight = ref(media?.matches ?? false)
  const onSystemChange = () => {
    systemPrefersLight.value = media?.matches ?? false
  }

  const current = computed<AppliedTheme>(() => {
    const sel = selection.value
    const custom = sel?.kind === 'custom' ? customThemes.value.find((t) => t.id === sel.id) : undefined
    if (custom) {
      return { themeId: custom.id, presetId: custom.base, scheme: PRESETS[custom.base].scheme, overrides: custom.overrides }
    }
    const presetId = sel?.kind === 'preset' ? sel.id : systemPrefersLight.value ? SYSTEM_LIGHT_PRESET : DEFAULT_PRESET_ID
    return { themeId: presetId, presetId, scheme: PRESETS[presetId].scheme, overrides: {} }
  })

  let started = false

  /** Applies the current theme and keeps <html> + the boot record in step. Idempotent. */
  function start() {
    if (started) return
    started = true
    const applied = usePersistedState<AppliedTheme | null>(APPLIED_THEME_KEY, {
      version: STORAGE_VERSION,
      parse: () => null,
      fallback: null,
      storage,
    })
    watch(current, (theme) => applyTheme(env.root, theme), { immediate: true })
    // Following the OS: store null so the boot script asks matchMedia itself.
    watch(
      [current, selection],
      ([theme, sel]) => {
        applied.value = sel ? theme : null
      },
      { immediate: true },
    )
    // The OS listener exists only while the user has not chosen a theme.
    watch(
      selection,
      (sel) => {
        if (sel) media?.removeEventListener('change', onSystemChange)
        else media?.addEventListener('change', onSystemChange)
      },
      { immediate: true },
    )
  }

  function saveCustomTheme(input: unknown, options: { acknowledgeContrast?: boolean } = {}): SaveThemeOutcome {
    const result = validateCustomTheme(input)
    if (!result.ok) return { result, saved: false }
    if (result.contrastFailures.length && !options.acknowledgeContrast) return { result, saved: false }
    const others = customThemes.value.filter((t) => t.id !== result.theme.id)
    customThemes.value = [...others, result.theme]
    return { result, saved: true }
  }

  function deleteCustomTheme(id: string) {
    const doomed = customThemes.value.find((t) => t.id === id)
    if (!doomed) return
    const sel = selection.value
    if (sel?.kind === 'custom' && sel.id === id) selection.value = { kind: 'preset', id: doomed.base }
    customThemes.value = customThemes.value.filter((t) => t.id !== id)
  }

  function exportTheme(id: string): string | null {
    const theme = customThemes.value.find((t) => t.id === id)
    if (!theme) return null
    return JSON.stringify({ ...theme, overrides: exportable(theme.overrides) }, null, 2)
  }

  function importTheme(json: string, options: { acknowledgeContrast?: boolean } = {}): SaveThemeOutcome {
    let parsed: unknown
    try {
      parsed = JSON.parse(json)
    } catch {
      return { result: { ok: false, errors: ['the file is not valid JSON'] }, saved: false }
    }
    return saveCustomTheme(parsed, options)
  }

  return {
    presets: PRESET_IDS.map((id) => PRESETS[id]),
    customThemes,
    selection,
    current,
    start,
    selectPreset: (id: PresetId) => {
      selection.value = { kind: 'preset', id }
    },
    selectCustom: (id: string) => {
      if (customThemes.value.some((t) => t.id === id)) selection.value = { kind: 'custom', id }
    },
    followSystem: () => {
      selection.value = null
    },
    saveCustomTheme,
    deleteCustomTheme,
    exportTheme,
    importTheme,
  }
}

let instance: ReturnType<typeof createThemeController> | undefined

export function useTheme() {
  instance ??= createThemeController({
    storage: browserStorage(),
    root: document.documentElement,
    matchMedia: typeof window.matchMedia === 'function' ? window.matchMedia.bind(window) : undefined,
  })
  return instance
}
