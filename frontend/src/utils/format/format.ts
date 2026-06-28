/**
 * String formatting utilities shared across the frontend.
 * Every truncation, preview, or display-format helper should live here
 * — never duplicated inline in components.
 */

const STRIP_BRACES = /^{|}$/g
const NEWLINES = /\n/g

/** Truncate text to max characters, appending "..." if trimmed. */
export const truncate = (text: string, max = 50): string =>
  text.length <= max ? text : text.substring(0, max) + "..."

/** Normalize a JSON string by re-serializing, then strip the outer `{}` braces. */
const stripBraces = (json: string): string => json.replace(STRIP_BRACES, "")

/** Return a short single-line preview of a JSON string (e.g. tool call args). */
export const jsonPreview = (json: string, max = 50): string => {
  try {
    const parsed = JSON.parse(json)
    return truncate(stripBraces(JSON.stringify(parsed)), max)
  } catch {
    return truncate(json, max)
  }
}

/** Escape HTML special characters in a string. */
export const escapeHtml = (s: string): string =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

/** Strip dangerous HTML tags that could leak into the page layout or execute scripts. */
export const stripDangerousTags = (s: string): string =>
  s.replace(/<style[^>]*>[\s\S]*?<\/style>/gi, '')
   .replace(/<script[^>]*>[\s\S]*?<\/script>/gi, '')
   .replace(/<link[^>]*>/gi, '')
   .replace(/<meta[^>]*>/gi, '')
   .replace(/<title[^>]*>[\s\S]*?<\/title>/gi, '')

/** Extract display text from a tool result value, sanitizing HTML for safe v-html rendering. */
export const formatToolResultValue = (val: any): string => {
  if (typeof val === "string") return escapeHtml(stripDangerousTags(val))
  if (typeof val?.content === "string") return escapeHtml(stripDangerousTags(val.content))
  return val ? escapeHtml(stripDangerousTags(JSON.stringify(val, null, 2))) : ""
}

/** Check if a tool result value is an object (has structured data beyond a plain string). */
export const hasStructuredData = (val: any): boolean => {
  if (typeof val?.content === "string") return true
  return typeof val === "object" && val !== null
}

/** Serialize a tool result value as formatted JSON for raw display. */
export const formatToolResultRaw = (val: any): string =>
  val ? JSON.stringify(val, null, 2) : ""

/** Return a short preview of a tool result for inline display. */
export const resultPreview = (
  result: string | object,
  error?: string,
  max = 50,
): string => {
  if (error) return truncate(error, max)
  const str = typeof result === "string" ? result : JSON.stringify(result)
  if (!str || str === "{}") return "Finished successfully"
  return truncate(stripBraces(str).replace(NEWLINES, " "), max)
}
