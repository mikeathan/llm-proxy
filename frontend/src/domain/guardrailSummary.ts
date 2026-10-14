import { networkPostureLabel } from "../utils/network";
import type { AgentGuardrailsConfig, GuardrailSection } from "../types/admin";

// One line that says what a guardrail section currently does, so a folded
// section can still be read. Takes the policy that applies (for a workspace,
// the merged one), never the raw layer.
const SEPARATOR = " · ";
const OFF = "Off";

const count = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`;
const join = (parts: (string | null)[]) => parts.filter((p): p is string => !!p).join(SEPARATOR);
const listed = (value: string[] | undefined, word: string) => (value?.length ? count(value.length, word) : null);

export function guardrailSectionSummary(section: GuardrailSection, policy: AgentGuardrailsConfig): string {
  switch (section) {
    case "global": {
      const { block_secrets, user_blocked_patterns } = policy.global;
      return join([block_secrets ? "Blocking secrets" : "Not blocking secrets", listed(user_blocked_patterns, "blocked pattern")]);
    }
    case "terminal": {
      const t = policy.terminal;
      if (!t.enabled) return OFF;
      return join([listed(t.allowed_commands, "allowed command"), t.timeout_seconds > 0 ? `timeout ${t.timeout_seconds}s` : null]);
    }
    case "filesystem": {
      const f = policy.filesystem;
      if (!f.enabled) return OFF;
      return join([f.read_only ? "Read-only" : "Read and write", listed(f.blocked_filenames, "blocked file name")]);
    }
    case "communication": {
      const c = policy.communication;
      if (!c.enabled) return OFF;
      return c.require_review ? "Each notification needs approval" : "Notifications sent without approval";
    }
    case "network": {
      const n = policy.network;
      if (!n.enabled) return OFF;
      return join([networkPostureLabel(n.allow_lan_access, n.allow_internet_access, true), listed(n.blocked_domains, "blocked domain")]);
    }
    case "search":
      return "";
  }
}
