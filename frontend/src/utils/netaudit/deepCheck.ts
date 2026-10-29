import type { DeepCheck, DeepOutcomeState } from "../../types/netaudit"

/** How long a device is left alone after a deep check (the server enforces it: platform netaudit.DeepCooldown). */
export const DEEP_COOLDOWN_MS = 10 * 60_000

/**
 * What a deep check came to, in words. "Nothing wrong" is said only when a TLS or an SSH service answered and was judged;
 * a device with none of them gives the scripts nothing to look at, and that is not the same as being fine.
 */
export function deepOutcome(check: DeepCheck): { state: DeepOutcomeState; text: string } {
  const n = check.findings.length
  if (n > 0) return { state: "problems", text: `${n} ${n === 1 ? "problem" : "problems"} found.` }
  const ssh = check.ports.some((p) => p.ssh_answered)
  const tls = check.ports.some((p) => p.tls_answered)
  if (!ssh && !tls) {
    return { state: "nothing", text: "No TLS or SSH service answered, so there was nothing to check. This is not the same as nothing wrong." }
  }
  const kinds = [ssh ? "SSH" : "", tls ? "TLS" : ""].filter(Boolean)
  return { state: "clean", text: `Checked the ${kinds.join(" and ")} ${kinds.length === 1 ? "service" : "services"}. Nothing wrong was found.` }
}

/** How much of the cool-down is left, from the stored time of the last check; 0 when there is none or it has passed. */
export function cooldownLeftMs(lastAt: string | undefined, now: number): number {
  if (!lastAt) return 0
  const at = Date.parse(lastAt)
  if (Number.isNaN(at)) return 0
  return Math.max(0, DEEP_COOLDOWN_MS - (now - at))
}

export function cooldownText(leftMs: number): string {
  const minutes = Math.max(1, Math.ceil(leftMs / 60_000))
  return `Checked a moment ago. Try again in ${minutes} ${minutes === 1 ? "minute" : "minutes"}.`
}

/** What a refused or failed deep check means for the operator, from the status the server answered with. */
export function describeDeepError(status: number, message: string): string {
  if (status === 409 && /no open port/i.test(message)) return "The last scan found no open port on this device. Run the audit again, then try once more."
  if (status === 409) return "Another scan is running. Try again when it has finished."
  if (status === 503) return `The deep check cannot run: ${message}`
  if (status === 504) return "The deep check ran out of time on this device, and nothing was saved. The device may be slow or busy; try again later."
  if (status === 403) return "Only you can start a deep check, from this page, after confirming your passphrase."
  return message
}
