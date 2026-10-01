import type { RouteLocationRaw } from 'vue-router'

// DialogType backs confirm/prompt dialogs (useConfirm + ConfirmDialog).
export type DialogType = 'info' | 'warning' | 'error'

// BannerSeverity is the shared severity set for app/model banners. The
// superset covers useAppBanner ('critical' | 'notice' | 'error') and the
// model-banner subset ('critical' | 'notice'); both import this single union.
export type BannerSeverity = 'critical' | 'notice' | 'error'

// BannerAction deep-links from a banner action button to a route.
export interface BannerAction {
  // Label for the action button. Clicking it navigates to `to`, built with the
  // typed route builders (router/routes.ts), e.g. toSettings('local').
  label: string
  to: RouteLocationRaw
}

// AppBannerMessage is a fully-formed banner payload for the shared banner bus.
export interface AppBannerMessage {
  severity: BannerSeverity
  // Plain-text fallback. Prefer `html` when the message needs inline links or
  // emphasis; `html` is always internally generated (never user input).
  message: string
  // Optional HTML content. Rendered via v-html; must never contain untrusted
  // input (CONSTITUTION/security rules forbid v-html on user data, but this is
  // app-controlled content only).
  html?: string
  // When true the banner is persistent (no dismiss button) and represents a
  // standing state (e.g. a configuration warning). When false it is a transient
  // notification the user may dismiss.
  persistent?: boolean
  // Optional action button that deep-links to a route (e.g. a Settings section).
  action?: BannerAction
}

// Breakpoint names the viewport widths in theme/breakpoints.ts.
export type Breakpoint = 'sm' | 'md' | 'lg' | 'xl' | '2xl'

// The Workspaces main pane: the workspace overview (run pulse), a run's
// details, the file editor, the assistant, memory (list or entry), security
// or the playbook library.
export type WorkspaceMainView = 'overview' | 'history' | 'editor' | 'assistant' | 'memory' | 'memory-detail' | 'settings' | 'playbooks'

// StatusState is the fixed set of run / health states a StatusTag shows; the
// state → token styling lives in one place (constants/status.ts).
export type StatusState = 'success' | 'running' | 'queued' | 'error' | 'info' | 'neutral'

// BaseButton variants and sizes (Phase 5 primitives).
export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
export type ButtonSize = 'sm' | 'md' | 'lg'

// A DataTable column: `value` reads the cell text from a row; a `cell-<key>`
// slot overrides it. Numeric columns are right-aligned tabular numerals.
export interface DataTableColumn<T> {
  key: string
  label: string
  numeric?: boolean
  value?: (row: T) => string | number | null | undefined
}

// One choice in a SegmentedControl or SelectInput.
export interface ChoiceOption {
  value: string
  label: string
}

// ToastType backs the transient toast notifications (useToast).
export type ToastType = 'success' | 'error' | 'info' | 'warning'

/** Envelope for every persisted value: a schema version plus the data (D3). */
export interface PersistedEnvelope {
  version: number
  data: unknown
}

export interface PersistedStateOptions<T> {
  version: number
  /** Returns the value, or null when the stored data is unusable. */
  parse: (data: unknown) => T | null
  fallback: T
  /** Upgrades data stored under an older version; null discards it. */
  migrate?: (fromVersion: number, data: unknown) => T | null
  storage?: Storage
}
