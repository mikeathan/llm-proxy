// Centralized mapping of icon/symbol constants.
//
// RULE: Every emoji, SVG icon, or Unicode symbol used in the UI must be
// defined here and referenced by name — never hardcoded in templates,
// composables, or utility files.
//
// WHY: Hardcoded emojis are invisible to search, cannot be updated
// consistently, and often have variation-selector inconsistencies (⚠ vs ⚠️).
// Centralising ensures a single source of truth for every visual symbol.
//
// Adding a new icon:
//   1. Add the constant here with a descriptive name.
//   2. Import it where needed (never inline the raw character).
//   3. If the icon is an emoji used in multiple contexts, add a helper
//      function here (e.g. getPhaseMessage, getEventIcon).
//
// Adding a new SVG icon:
//   1. Add the SVG file to assets/svg/ and register it in assets/svg/index.ts.
//   2. If the icon needs dynamic sizing or theming, add a case to UIIcon.vue. 
//   3. If it's consumed by two or more components, add a named constant here.

// ── Lifecycle phase icons ──
export const LIFECYCLE_ICONS: Record<string, string> = {
  stuck_detected: "⚠️",
  fallback_started: "🔄",
  fallback_waiting: "⏳",
  fallback_completed: "✅",
}

// ── Tool event emojis for plain-text formatting (dispatcher.ts text export) ──
// These are NOT used in Vue templates — UI tool calls/results use SVG icons
// from assets/svg/ via Icon.vue.  These are only referenced by getEventIcon()
// for plain-text copy/paste output.
export const TEXT_EVENT_TOOL_CALL = "🛠️"
export const TEXT_EVENT_TOOL_RESULT = "✅"
export const TEXT_EVENT_GUARDRAIL_BLOCKED = "🛑"

// ── Common UI icons ──
export const CHAT_ICON = "💬"
export const SEARCH_ICON = "🔍"
export const WARNING_ICON = "⚠️"   // with variation selector-16
export const ERROR_ICON = "❌"
export const CHECK_ICON = "✅"

// ── Toast notification symbols (Unicode text, not emoji) ──
export const TOAST_SUCCESS = "✓"
export const TOAST_ERROR = "✕"
export const TOAST_WARNING = "⚠"    // without variation selector — cleaner in toast
export const TOAST_INFO = "ℹ"
export const TOAST_CLOSE = "×"

// ── Phase message formatter ──
export const getPhaseMessage = (phase: string, payload: Record<string, any>): string => {
  switch (phase) {
    case "stuck_detected":
      return `${LIFECYCLE_ICONS[phase]} Model stuck in reasoning loop (${payload.reasoning_chars} chars) — retrying...`
    case "fallback_started":
      return `${LIFECYCLE_ICONS[phase]} Switching to ${payload.mode || "fallback"} mode — ${payload.reason || ""}`
    case "fallback_waiting":
      return `${LIFECYCLE_ICONS[phase]} Waiting for non-streaming response... (elapsed: ${payload.elapsed || "0s"})`
    case "fallback_completed":
      return `${LIFECYCLE_ICONS[phase]} Fallback completed successfully`
    case "completed":
      return ``
    default:
      return ""
  }
}

// ── Event-to-icon mapping used by text formatters ──
export const getEventIcon = (type: string): string => {
  switch (type) {
    case "tool_call": return TEXT_EVENT_TOOL_CALL
    case "tool_result": return TEXT_EVENT_TOOL_RESULT
    case "guardrail_violation": return TEXT_EVENT_GUARDRAIL_BLOCKED
    case "guardrail_blocked": return TEXT_EVENT_GUARDRAIL_BLOCKED
    case "lifecycle": return ""
    default: return ""
  }
}
