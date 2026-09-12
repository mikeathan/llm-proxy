// Shared error-to-message coercion. Components/services must never read
// `error.message` off an untyped `any`; normalize with errorMessage() instead
// so UI copy ("cause + next action") can rely on a string.
export const UNKNOWN_ERROR_MESSAGE = 'Unexpected error'

export function errorMessage(error: unknown, fallback: string = UNKNOWN_ERROR_MESSAGE): string {
  if (error instanceof Error && error.message) return error.message
  if (typeof error === 'string' && error) return error
  return fallback
}
