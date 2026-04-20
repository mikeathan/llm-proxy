/**
 * Generates a unique identifier.
 * Uses crypto.randomUUID if available, otherwise falls back to a random string.
 */
export const generateId = (): string => {
  return typeof crypto !== "undefined" && crypto.randomUUID
    ? crypto.randomUUID()
    : Math.random().toString(36).substring(2, 11);
};
