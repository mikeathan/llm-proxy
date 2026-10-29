import type { SessionLength } from "../../types/auth"

/** What is wrong with the two limits a person typed, or "" when they can be saved (the server checks the same rules). */
export function sessionLengthProblem(idleHours: number, absoluteDays: number, bounds: SessionLength): string {
  if (!Number.isInteger(idleHours) || idleHours < 1 || idleHours > bounds.max_idle_hours) {
    return `The idle limit must be a whole number of hours from 1 to ${bounds.max_idle_hours}.`
  }
  if (!Number.isInteger(absoluteDays) || absoluteDays < 1 || absoluteDays > bounds.max_absolute_days) {
    return `The absolute limit must be a whole number of days from 1 to ${bounds.max_absolute_days}.`
  }
  if (idleHours > absoluteDays * 24) return "The idle limit cannot be longer than the absolute limit."
  return ""
}
