import { ref } from 'vue'
import type { SettingsTab } from '../../types/admin'

export type BannerSeverity = 'critical' | 'notice' | 'error'

export interface BannerAction {
  // Label for the action button. Clicking it navigates to a Settings tab
  // (and switches the main view to Settings) so callers can deep-link to the
  // relevant configuration page.
  label: string
  settingsTab: SettingsTab
}

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
  // notification the user may dismiss. This is presentation-only — precedence
  // between messages is owned by the emitting component, not this bus.
  persistent?: boolean
  // Optional action button that deep-links to a Settings page.
  action?: BannerAction
}

// Generic, domain-agnostic banner bus. It is a pure sink: any part of the UI
// may `show` a fully-formed message and the bus exposes the latest one as
// `active` for AppBanner.vue to render. Precedence between messages (e.g. a
// transient error overriding a persistent warning) is coordinated by the
// emitting components, not by this composable.
const active = ref<AppBannerMessage | null>(null)

export function useAppBanner() {
  const show = (msg: AppBannerMessage) => {
    active.value = msg
  }
  const clear = () => {
    active.value = null
  }
  return { active, show, clear }
}
