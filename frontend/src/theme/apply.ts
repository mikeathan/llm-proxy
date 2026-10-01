import type { AppliedTheme } from '../types/theme'
import { TOKEN_NAMES } from './tokenRegistry'

// Writes a theme onto <html>: the preset's stylesheet block via data-theme, the
// `dark` class (kept for prose-invert and third-party content, D16), and a
// custom theme's validated overrides as inline custom properties. Idempotent:
// overrides from a previous theme are always cleared first.
// public/theme-boot.js mirrors this for the pre-paint step (contract-tested).

const DARK_CLASS = 'dark'

export function applyTheme(root: HTMLElement, theme: AppliedTheme): void {
  for (const name of TOKEN_NAMES) root.style.removeProperty(`--${name}`)
  for (const [name, value] of Object.entries(theme.overrides)) root.style.setProperty(`--${name}`, value)
  if (!root.style.length) root.removeAttribute('style')
  root.dataset.theme = theme.presetId
  root.classList.toggle(DARK_CLASS, theme.scheme === 'dark')
}
