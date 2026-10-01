// Reduced motion for scripted scrolling (Phase 6). CSS motion is handled by the
// motion tokens and the global safety net in style.css; a smooth scroll started
// from script is not, so every smooth scroll goes through here.
const REDUCED_MOTION = '(prefers-reduced-motion: reduce)'

export function prefersReducedMotion(): boolean {
  return typeof globalThis.matchMedia === 'function' && globalThis.matchMedia(REDUCED_MOTION).matches
}

/** A smooth scroll becomes an immediate one when the user asks for reduced motion. */
export function motionScroll(behavior: ScrollBehavior): ScrollBehavior {
  return behavior === 'smooth' && prefersReducedMotion() ? 'auto' : behavior
}
