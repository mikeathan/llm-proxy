import type { SettingsTab } from './admin'

// DialogType backs confirm/prompt dialogs (useConfirm + ConfirmDialog).
export type DialogType = 'info' | 'warning' | 'error'

// BannerSeverity is the shared severity set for app/model banners. The
// superset covers useAppBanner ('critical' | 'notice' | 'error') and the
// model-banner subset ('critical' | 'notice'); both import this single union.
export type BannerSeverity = 'critical' | 'notice' | 'error'

// BannerAction deep-links to a Settings tab from a banner action button.
export interface BannerAction {
  // Label for the action button. Clicking it navigates to a Settings tab
  // (and switches the main view to Settings) so callers can deep-link to the
  // relevant configuration page.
  label: string
  settingsTab: SettingsTab
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
  // Optional action button that deep-links to a Settings page.
  action?: BannerAction
}

// ToastType backs the transient toast notifications (useToast).
export type ToastType = 'success' | 'error' | 'info' | 'warning'
