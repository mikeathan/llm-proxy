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

type Summarise = (policy: AgentGuardrailsConfig) => string;

const SUMMARISERS: Record<GuardrailSection, Summarise> = {
  global: ({ global }) =>
    join([global.block_secrets ? "Blocking secrets" : "Not blocking secrets", listed(global.user_blocked_patterns, "blocked pattern")]),
  terminal: ({ terminal: t }) =>
    t.enabled ? join([listed(t.allowed_commands, "allowed command"), t.timeout_seconds > 0 ? `timeout ${t.timeout_seconds}s` : null]) : OFF,
  filesystem: ({ filesystem: f }) =>
    f.enabled ? join([f.read_only ? "Read-only" : "Read and write", listed(f.blocked_filenames, "blocked file name")]) : OFF,
  communication: ({ communication: c }) =>
    !c.enabled ? OFF : c.require_review ? "Each notification needs approval" : "Notifications sent without approval",
  network: ({ network: n }) =>
    n.enabled
      ? join([networkPostureLabel(n.allow_lan_access, n.allow_internet_access, true), listed(n.blocked_domains, "blocked domain")])
      : OFF,
  search: () => "",
};

export const guardrailSectionSummary = (section: GuardrailSection, policy: AgentGuardrailsConfig): string =>
  SUMMARISERS[section](policy);
