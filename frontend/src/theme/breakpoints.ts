import type { Breakpoint } from '../types/ui'

// Viewport breakpoints in px — the single source for Tailwind's `screens`
// (theme/tailwindTokens.ts) and useResponsiveLayout. Values are Tailwind's
// defaults, so existing `md:` / `lg:` classes keep their meaning.
export const BREAKPOINTS: Record<Breakpoint, number> = {
  sm: 640,
  md: 768,
  lg: 1024,
  xl: 1280,
  '2xl': 1536,
}

/** Below this the shell shows one pane and the sidebar becomes a drawer. */
export const DESKTOP_BREAKPOINT: Breakpoint = 'lg'
