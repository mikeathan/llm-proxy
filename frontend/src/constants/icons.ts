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

// ── Tool event emojis for plain-text formatting ──
// Only the two still rendered as text remain; the per-event `getEventIcon`
// mapper was removed with its only consumer (the plain-text event log export).
export const TEXT_EVENT_TOOL_RESULT = "✅"
export const TEXT_EVENT_GUARDRAIL_BLOCKED = "🛑"

// ── Common UI icons ──
export const FOLDER_ICON = "📁"

// ── Toast notification symbols (Unicode text, not emoji) ──
export const TOAST_SUCCESS = "✓"
export const TOAST_ERROR = "✕"
export const TOAST_WARNING = "⚠"    // without variation selector — cleaner in toast
export const TOAST_INFO = "ℹ"
export const TOAST_CLOSE = "×"
