// Theme system types (plan: docs/PLANS/cross-cutting/frontend-redesign-retro.md,
// D1, D2, D15). The single home for theme types — theme/, composables/ and
// components import them from here.

/** Every token declared in styles/tokens.css. The registry maps each one. */
export type TokenName =
  // Colour — per theme
  | 'canvas'
  | 'surface'
  | 'surface-raised'
  | 'surface-hover'
  | 'surface-active'
  | 'border-hairline'
  | 'border-strong'
  | 'border-control'
  | 'text-primary'
  | 'text-secondary'
  | 'text-muted'
  | 'text-faint'
  | 'text-decorative'
  | 'text-inverse'
  | 'text-on-accent'
  | 'accent-brand'
  | 'accent-info'
  | 'accent-info-text'
  | 'state-success'
  | 'state-running'
  | 'state-queued'
  | 'state-error'
  | 'state-live'
  | 'focus-ring'
  | 'bar-track'
  | 'scrim'
  // Typography, shape, motion — theme-independent (D17)
  | 'font-sans'
  | 'font-mono'
  | 'text-micro'
  | 'tracking-micro'
  | 'text-body'
  | 'text-small'
  | 'text-heading'
  | 'text-display'
  | 'text-numeral'
  | 'leading-body'
  | 'radius-sm'
  | 'radius-md'
  | 'offset-control'
  | 'space-unit'
  | 'motion-fast'
  | 'motion-base'
  | 'motion-slow'
  | 'ease-standard'
  | 'ease-emphasised'

/** Value grammar a token accepts. Colours are stored as "R G B" channels (D15). */
export type TokenKind = 'colour' | 'length' | 'duration' | 'font' | 'easing' | 'number'

/** 'theme' tokens vary per preset; 'global' tokens are shared by every preset. */
export type TokenScope = 'theme' | 'global'

export interface TokenSpec {
  kind: TokenKind
  scope: TokenScope
  /** Whether a custom theme may override it (D2: fonts and easings are not). */
  editable: boolean
}

/** An opaque sRGB colour as integer channels 0–255. */
export type Channels = readonly [number, number, number]

export type ColourScheme = 'dark' | 'light'

export type PresetId = 'retro-dark' | 'retro-dark-soft' | 'retro-dark-lifted' | 'retro-paper'

/** A partial token map in canonical stored form (colour → "R G B"). */
export type ThemeTokens = Partial<Record<TokenName, string>>

/** A complete token map: every token resolved. */
export type ResolvedTokens = Record<TokenName, string>

/** Token declarations read from styles/tokens.css, before presets are resolved. */
export interface TokenSheet {
  /** Theme-independent tokens (the plain `:root` block). */
  global: ThemeTokens
  /** Tokens each preset block declares (retro-dark includes the `:root` defaults). */
  themes: Record<PresetId, ThemeTokens>
  /** `color-scheme` each preset block declares, if any. */
  schemes: Partial<Record<PresetId, ColourScheme>>
}

/** A built-in preset: fully resolved and proven by the preset tests. */
export interface ThemeDefinition {
  id: PresetId
  label: string
  scheme: ColourScheme
  tokens: ResolvedTokens
}

/** A user theme: a validated partial merged over a base preset (D2). */
export interface CustomTheme {
  id: string
  label: string
  base: PresetId
  overrides: ThemeTokens
}

export type ContrastRole =
  | 'text'
  | 'state text'
  | 'control boundary'
  | 'focus indicator'
  | 'fill / large text'
  | 'primary button'
  | 'text on accent'

export interface ContrastPair {
  fg: TokenName
  bg: TokenName
  min: number
  role: ContrastRole
}

export interface ContrastResult extends ContrastPair {
  ratio: number
  pass: boolean
}

/**
 * Result of validating an imported custom theme. Grammar and name errors reject
 * the theme; contrast failures are returned for the editor to warn about (D2).
 */
export type ThemeValidationResult =
  | { ok: true; theme: CustomTheme; contrastFailures: ContrastResult[] }
  | { ok: false; errors: string[] }

/** What the user picked. `null` in storage means "follow the OS scheme". */
export type ThemeSelection = { kind: 'preset'; id: PresetId } | { kind: 'custom'; id: string }

/**
 * The theme as applied to <html>: a preset stylesheet block (data-theme) plus
 * validated inline overrides for a custom theme. It is also the record the
 * pre-paint boot script reads (D19), so it holds only validated values.
 */
export interface AppliedTheme {
  themeId: string
  presetId: PresetId
  scheme: ColourScheme
  overrides: ThemeTokens
}

/** Everything the theme controller touches outside itself — injectable for tests. */
export interface ThemeEnvironment {
  storage: Storage | undefined
  root: HTMLElement
  matchMedia: ((query: string) => MediaQueryList) | undefined
}

export interface SaveThemeOutcome {
  result: ThemeValidationResult
  saved: boolean
}

/**
 * What the theme editor edits (Settings · Appearance): every editable token's
 * value as the user sees it — colours as hex, lengths / durations as typed —
 * over a fixed base preset. Saving turns it back into a CustomTheme input.
 */
export interface ThemeDraft {
  id: string
  label: string
  base: PresetId
  values: Partial<Record<TokenName, string>>
}

/** A custom theme as the editor hands it to the validator: raw, unnormalised overrides. */
export interface ThemeInput {
  id: string
  label: string
  base: PresetId
  overrides: Record<string, string>
}
