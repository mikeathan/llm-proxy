/**
 * String formatting utilities shared across the frontend.
 * Every truncation, preview, or display-format helper should live here
 * — never duplicated inline in components.
 *
 * Only escapeHtml remains live today (used by the model banner). The JSON
 * preview / tool-result formatters used to live here but had no consumers —
 * current tool-result rendering lives in utils/message/messageBuilder.ts.
 */

/** Escape HTML special characters in a string. */
export const escapeHtml = (s: string): string =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
