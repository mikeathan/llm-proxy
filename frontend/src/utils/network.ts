// Network posture copy shared by the workspace guardrail form and the
// automation grant select, so both surfaces describe the same reachability in
// the same words. Pure mapping — no component state.

export const NETWORK_POSTURE = {
  none: "No network",
  lan: "Local network only",
  internetOnly: "Internet only (no local network)",
  internet: "Local network + Internet",
} as const;

export const NETWORK_LAN_HELP =
  "Local network = devices on this LAN (e.g. 192.168.x, 10.x) — internal services and network scanning.";

export const NETWORK_INTERNET_HELP =
  "Internet = public hosts (websites, APIs, search).";

// Applies to the in-process agent tools only: once any network is allowed, a
// terminal command still has direct network access (cannot be split at the OS
// layer). See the shell note next to the toggles.
export const NETWORK_SHELL_CAVEAT =
  "Controls the agent's network tools (fetch, scan, search). Terminal commands keep direct network access once any network is allowed.";

export function networkPostureLabel(
  lan: boolean,
  internet: boolean,
  enabled = true,
): string {
  if (!enabled) return NETWORK_POSTURE.none;
  if (lan && internet) return NETWORK_POSTURE.internet;
  if (internet) return NETWORK_POSTURE.internetOnly;
  if (lan) return NETWORK_POSTURE.lan;
  return NETWORK_POSTURE.none;
}
