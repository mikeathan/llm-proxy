import type { AgentGuardrailsConfig, GuardrailMergeRule, GuardrailSection, GuardrailSource } from "../types/admin";

/**
 * Workspace guardrail layers. The backend merges a workspace's layer over the
 * global policy field by field (backend/models/config.go → MergeWith); this
 * mirrors those rules so the workspace page can show each effective value and
 * where it comes from. A shared fixture runs against both
 * (__TESTS__/fixtures/guardrailMerge.*.json) so the mirror cannot drift.
 */

const on = (loosens: boolean): GuardrailMergeRule => ({ kind: "on-wins", loosens });
const union = (loosens: boolean): GuardrailMergeRule => ({ kind: "union", loosens });
const positive: GuardrailMergeRule = { kind: "positive", loosens: false };
const present: GuardrailMergeRule = { kind: "present", loosens: true };

export const GUARDRAIL_RULES: Readonly<Record<GuardrailSection, Readonly<Record<string, GuardrailMergeRule>>>> = {
  global: { block_secrets: on(false), user_blocked_patterns: union(false) },
  terminal: {
    enabled: on(true),
    timeout_seconds: positive,
    session_idle_timeout_seconds: { kind: "layer", loosens: false },
    max_output_size_chars: positive,
    allowed_commands: union(true),
    allowed_env_vars: union(true),
    blocked_patterns: union(false),
    path_extensions: union(true),
    allowed_external_paths: union(true),
  },
  filesystem: {
    enabled: on(true),
    read_only: on(false),
    max_file_size_kb: positive,
    allowed_paths: union(true),
    allowed_extensions: union(true),
    blocked_filenames: union(false),
  },
  search: { enabled: on(true), max_query_len: positive, blocked_sites: union(false) },
  communication: {
    enabled: on(true),
    max_messages_per_task: positive,
    require_review: { kind: "off-wins", loosens: true },
  },
  network: {
    enabled: present,
    allow_lan_access: present,
    allow_internet_access: present,
    max_fetch_size_kb: positive,
    timeout_seconds: positive,
    blocked_domains: union(false),
    blocked_ips: union(false),
  },
};

const SECTIONS = Object.keys(GUARDRAIL_RULES) as GuardrailSection[];

type Fields = Record<string, unknown>;
const fieldsOf = (cfg: AgentGuardrailsConfig | null | undefined, section: GuardrailSection): Fields | undefined =>
  cfg?.[section] as unknown as Fields | undefined;
const list = (value: unknown): string[] => (Array.isArray(value) ? (value as string[]) : []);
const num = (value: unknown): number => (typeof value === "number" ? value : 0);
const unique = (items: string[]) => [...new Set(items)];
const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

function mergeField(rule: GuardrailMergeRule, base: unknown, layer: unknown, layerHasSection: boolean): unknown {
  switch (rule.kind) {
    case "union":
      return unique([...list(base), ...list(layer)]);
    case "on-wins":
      return layer === true ? true : base;
    case "off-wins":
      return layer === false ? false : base;
    case "positive":
      return num(layer) > 0 ? layer : base;
    case "layer":
      return num(layer);
    case "present":
      return layerHasSection ? layer === true : base;
  }
}

/** The effective policy of a workspace: its layer (if any) merged over the global policy. */
export function mergeGuardrails(global: AgentGuardrailsConfig, layer: AgentGuardrailsConfig | null): AgentGuardrailsConfig {
  const out = clone(global);
  if (!layer) return out;
  for (const section of SECTIONS) {
    const target = fieldsOf(out, section) ?? {};
    const own = fieldsOf(layer, section);
    for (const [field, rule] of Object.entries(GUARDRAIL_RULES[section])) {
      target[field] = mergeField(rule, target[field], own?.[field], own !== undefined);
    }
    (out as unknown as Record<string, Fields>)[section] = target;
  }
  return out;
}

/**
 * A layer that changes nothing yet: switches off, lists empty, numbers 0 —
 * except the fields a layer always sets (session idle, network access) and the
 * review switch it can only turn off, which start at the global values.
 */
export function seedLayer(global: AgentGuardrailsConfig): AgentGuardrailsConfig {
  const layer: Record<string, Fields> = {};
  for (const section of SECTIONS) {
    const fields: Fields = {};
    for (const [field, rule] of Object.entries(GUARDRAIL_RULES[section])) {
      const inherited = fieldsOf(global, section)?.[field];
      if (rule.kind === "union") fields[field] = [];
      else if (rule.kind === "positive") fields[field] = 0;
      else if (rule.kind === "on-wins") fields[field] = false;
      else if (rule.kind === "off-wins") fields[field] = true;
      else if (rule.kind === "layer") fields[field] = num(inherited);
      else fields[field] = inherited === true;
    }
    layer[section] = fields;
  }
  return layer as unknown as AgentGuardrailsConfig;
}

/**
 * A stored layer as only its own contribution: repeats of the global policy
 * (the old editor saved a full copy) and missing sections become "inherit",
 * without changing what the layer means.
 */
export function normalizeLayer(stored: AgentGuardrailsConfig, global: AgentGuardrailsConfig): AgentGuardrailsConfig {
  const layer = seedLayer(global) as unknown as Record<string, Fields>;
  for (const section of SECTIONS) {
    const own = fieldsOf(stored, section);
    if (!own) continue;
    const base = fieldsOf(global, section);
    const fields = layer[section]!;
    for (const [field, rule] of Object.entries(GUARDRAIL_RULES[section])) {
      const value = own[field];
      const inherited = base?.[field];
      switch (rule.kind) {
        case "union":
          fields[field] = unique(list(value).filter((item) => !list(inherited).includes(item)));
          break;
        case "on-wins":
          fields[field] = value === true && inherited !== true;
          break;
        case "off-wins":
          fields[field] = value !== false;
          break;
        case "positive":
          fields[field] = num(value) > 0 && value !== inherited ? value : 0;
          break;
        case "layer":
          fields[field] = num(value);
          break;
        case "present":
          fields[field] = value === true;
          break;
      }
    }
  }
  return layer as unknown as AgentGuardrailsConfig;
}

/** Where a field's effective value comes from, for the workspace page's tags. */
export function fieldSource(
  section: GuardrailSection,
  field: string,
  global: AgentGuardrailsConfig,
  layer: AgentGuardrailsConfig | null,
): GuardrailSource {
  const rule = GUARDRAIL_RULES[section][field];
  if (!layer || !rule) return "inherited";
  const value = fieldsOf(layer, section)?.[field];
  const inherited = fieldsOf(global, section)?.[field];
  const changed = (() => {
    switch (rule.kind) {
      case "union":
        return list(value).some((item) => !list(inherited).includes(item));
      case "on-wins":
        return value === true && inherited !== true;
      case "off-wins":
        return value === false && inherited !== false;
      case "positive":
        return num(value) > 0 && value !== inherited;
      case "layer":
        return num(value) !== num(inherited);
      case "present":
        return (value === true) !== (inherited === true);
    }
  })();
  if (!changed) return "inherited";
  // A network switch loosens only when the workspace turns it on.
  const loosens = rule.kind === "present" ? value === true : rule.loosens;
  return loosens ? "exception" : "overridden";
}
