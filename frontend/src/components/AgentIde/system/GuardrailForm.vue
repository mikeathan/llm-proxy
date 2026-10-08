<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { AgentGuardrailsConfig, GuardrailFieldSpec, GuardrailSection, GuardrailSectionSpec, GuardrailSource } from "../../../types/admin";
import type { StatusState } from "../../../types/ui";
import { GUARDRAIL_RULES, fieldSource, mergeGuardrails } from "../../../domain/guardrailLayers";
import { NETWORK_INTERNET_HELP, NETWORK_LAN_HELP, NETWORK_SHELL_CAVEAT, networkPostureLabel } from "../../../utils/network";
import BaseToggle from "../../common/buttons/BaseToggle.vue";
import FormField from "../../common/forms/FormField.vue";
import ListField from "../../common/forms/ListField.vue";
import MicroLabel from "../../common/display/MicroLabel.vue";
import StatusTag from "../../common/display/StatusTag.vue";
import Callout from "../../common/feedback/Callout.vue";

// The guardrail policy form, in two modes. Without `inherited` it edits a whole
// policy (Settings · Guardrails). With `inherited` (the global policy) it edits
// a workspace's own layer and shows what the backend will actually apply
// (domain/guardrailLayers mirrors its merge): global list entries are fixed and
// the workspace adds to them, switches the global policy turns on stay on,
// an empty number inherits, and every field says whether it is inherited,
// overridden or an exception.
const props = defineProps<{
  modelValue: AgentGuardrailsConfig;
  inherited?: AgentGuardrailsConfig;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", config: AgentGuardrailsConfig): void;
}>();

const SESSION_IDLE = "session_idle_timeout_seconds";
const SESSION_IDLE_DEFAULT = 1800;
const EXTERNAL_PATHS = "allowed_external_paths";

const SECTIONS: GuardrailSectionSpec[] = [
  {
    section: "global",
    title: "Secrets and patterns",
    fields: [
      { field: "block_secrets", label: "Block secrets", control: "toggle", hint: "Redacts credentials and personal data from tool calls." },
      { field: "user_blocked_patterns", label: "Blocked patterns", control: "list", hint: "One per line. A tool call matching any of them is blocked." },
    ],
  },
  {
    section: "terminal",
    title: "Terminal tool",
    switchField: "enabled",
    fields: [
      { field: "allowed_commands", label: "Allowed commands", control: "list", placeholder: "ls\ngit" },
      { field: "blocked_patterns", label: "Blocked command patterns", control: "list", placeholder: "rm -rf" },
      { field: "allowed_env_vars", label: "Allowed environment variables", control: "list", placeholder: "PATH\nLANG" },
      { field: "path_extensions", label: "Workspace PATH extensions", control: "list", placeholder: "node_modules/.bin\n.venv/bin" },
      { field: EXTERNAL_PATHS, label: "Allowed external paths", control: "list", hint: "Paths outside the workspace directory the agent may reach.", placeholder: "/mnt/data" },
      { field: "timeout_seconds", label: "Command timeout (sec)", control: "number" },
      { field: "max_output_size_chars", label: "Max output (chars)", control: "number" },
    ],
  },
  {
    section: "filesystem",
    title: "File system tool",
    switchField: "enabled",
    fields: [
      { field: "read_only", label: "Read-only access", control: "toggle" },
      { field: "allowed_paths", label: "Allowed paths", control: "list", placeholder: "." },
      { field: "allowed_extensions", label: "Allowed extensions", control: "list", placeholder: ".md" },
      { field: "blocked_filenames", label: "Blocked file names", control: "list", placeholder: ".env" },
      { field: "max_file_size_kb", label: "Max file size (KB)", control: "number" },
    ],
  },
  {
    section: "communication",
    title: "Communication",
    switchField: "enabled",
    fields: [
      { field: "require_review", label: "Approve each notification", control: "toggle", hint: "Pause agent-initiated sends for approval. Turn off for unattended notifications." },
      { field: "max_messages_per_task", label: "Max messages per task", control: "number", hint: "Saved policy value; currently not enforced for agent notifications." },
    ],
  },
  {
    section: "network",
    title: "Agent network access",
    switchField: "enabled",
    fields: [
      { field: "allow_lan_access", label: "Local network (LAN)", control: "toggle", hint: NETWORK_LAN_HELP },
      { field: "allow_internet_access", label: "Internet", control: "toggle", hint: NETWORK_INTERNET_HELP },
      { field: "blocked_domains", label: "Blocked domains", control: "list", placeholder: "example.com" },
      { field: "blocked_ips", label: "Blocked IPs", control: "list", placeholder: "10.0.0.5" },
      { field: "max_fetch_size_kb", label: "Max fetch size (KB)", control: "number" },
      { field: "timeout_seconds", label: "Fetch timeout (sec)", control: "number" },
    ],
  },
];

const SOURCE_TAG: Record<GuardrailSource, { state: StatusState; label: string }> = {
  inherited: { state: "neutral", label: "Inherited" },
  overridden: { state: "info", label: "Overridden" },
  exception: { state: "running", label: "Exception" },
};

type Fields = Record<string, unknown>;
const copy = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;
const local = ref<AgentGuardrailsConfig>(copy(props.modelValue));

watch(
  () => props.modelValue,
  (value) => {
    if (JSON.stringify(value) !== JSON.stringify(local.value)) local.value = copy(value);
  },
  { deep: true },
);
watch(local, (value) => emit("update:modelValue", copy(value)), { deep: true });

const layered = computed(() => !!props.inherited);
const effective = computed(() => (props.inherited ? mergeGuardrails(props.inherited, local.value) : local.value));

const fields = (cfg: AgentGuardrailsConfig | undefined, section: GuardrailSection): Fields =>
  (cfg?.[section] as unknown as Fields | undefined) ?? {};
const own = (section: GuardrailSection, field: string) => fields(local.value, section)[field];
const base = (section: GuardrailSection, field: string) => fields(props.inherited, section)[field];
const shown = (section: GuardrailSection, field: string) => fields(effective.value, section)[field];
const asList = (value: unknown): string[] => (Array.isArray(value) ? (value as string[]) : []);
// Workspace entries the global policy already has (dropped on the next load).
const redundant = (section: GuardrailSection, field: string) =>
  layered.value ? asList(own(section, field)).filter((item) => asList(base(section, field)).includes(item)) : [];

function set(section: GuardrailSection, field: string, value: unknown) {
  const next = copy(local.value) as unknown as Record<string, Fields>;
  next[section] = { ...(next[section] ?? {}), [field]: value };
  local.value = next as unknown as AgentGuardrailsConfig;
}

const source = (section: GuardrailSection, field: string): GuardrailSource | null =>
  props.inherited ? fieldSource(section, field, props.inherited, local.value) : null;

/** A switch the global policy turns on cannot be turned off by a workspace. */
const lockedOn = (section: GuardrailSection, field: string) =>
  layered.value && GUARDRAIL_RULES[section][field]?.kind === "on-wins" && base(section, field) === true;

const lockedOff = (section: GuardrailSection, field: string) =>
  layered.value && GUARDRAIL_RULES[section][field]?.kind === "off-wins" && base(section, field) === false;

const toggleValue = (section: GuardrailSection, field: string) =>
  lockedOn(section, field) ? true : lockedOff(section, field) ? false : own(section, field) === true;

const sectionOpen = (spec: GuardrailSectionSpec) => !spec.switchField || shown(spec.section, spec.switchField) === true;

// Numbers: a workspace leaves them empty to inherit (0 means "inherit" there).
function numberValue(section: GuardrailSection, field: string): number | "" {
  const value = own(section, field);
  if (typeof value !== "number") return "";
  return layered.value && value === 0 ? "" : value;
}
function setNumber(section: GuardrailSection, field: string, raw: string) {
  const value = raw.trim() === "" ? 0 : Math.max(0, Math.floor(Number(raw)) || 0);
  set(section, field, value);
}
const numberPlaceholder = (section: GuardrailSection, field: string) =>
  layered.value ? `${String(base(section, field) ?? 0)} (global)` : undefined;

function fieldHint(section: GuardrailSection, spec: GuardrailFieldSpec): string | undefined {
  if (lockedOn(section, spec.field)) return "On in the global policy — a workspace cannot turn it off.";
  if (lockedOff(section, spec.field)) return "Off in the global policy — a workspace cannot turn it on.";
  if (layered.value && spec.control === "list") return "Added for this workspace; the global entries above always apply.";
  if (layered.value && spec.control === "number") return spec.hint ? `Empty uses the global value. ${spec.hint}` : "Empty uses the global value.";
  return spec.hint;
}

// Session idle: the one field a workspace layer always sets (backend rule).
const sessionIdle = computed(() => Number(own("terminal", SESSION_IDLE) ?? 0));
const setSessionIdle = (on: boolean) => set("terminal", SESSION_IDLE, on ? SESSION_IDLE_DEFAULT : 0);

const externalPaths = computed(() => asList(shown("terminal", EXTERNAL_PATHS)));
const networkPosture = computed(() =>
  networkPostureLabel(
    shown("network", "allow_lan_access") === true,
    shown("network", "allow_internet_access") === true,
    shown("network", "enabled") === true,
  ),
);
const tagOf = (section: GuardrailSection, field: string) => {
  const s = source(section, field);
  return s ? SOURCE_TAG[s] : null;
};
</script>

<template>
  <div class="grid gap-4 xl:grid-cols-2">
    <section
      v-for="spec in SECTIONS"
      :key="spec.section"
      :aria-label="spec.title"
      class="flex min-w-0 flex-col gap-4 rounded-[var(--radius-sm)] border border-hairline bg-canvas p-4"
    >
      <div class="flex flex-wrap items-center justify-between gap-2">
        <MicroLabel>{{ spec.title }}</MicroLabel>
        <span v-if="spec.switchField" class="flex items-center gap-2">
          <StatusTag v-if="tagOf(spec.section, spec.switchField)" :data-test="`source-${spec.section}-${spec.switchField}`" v-bind="tagOf(spec.section, spec.switchField)!" />
          <BaseToggle
            :model-value="toggleValue(spec.section, spec.switchField)"
            :disabled="lockedOn(spec.section, spec.switchField)"
            :label="spec.title"
            hide-label
            @update:model-value="set(spec.section, spec.switchField, $event)"
          />
        </span>
      </div>
      <p v-if="spec.section === 'communication'" class="m-0 text-[length:var(--text-small)] text-muted">
        Allow the agent to send through connectors configured in Settings · Communication, using notify_user. Enabling a connector alone does not grant this permission. Host network access must also be on.
      </p>
      <p v-if="spec.switchField && lockedOn(spec.section, spec.switchField)" class="m-0 text-[length:var(--text-small)] text-faint">
        On in the global policy — a workspace cannot turn it off.
      </p>

      <template v-if="sectionOpen(spec)">
        <template v-if="spec.section === 'network'">
          <p class="m-0 text-[length:var(--text-small)] text-muted">
            The two switches are independent: Internet does not include the local network.
            Agent tools can reach: <span class="font-mono text-primary">{{ networkPosture }}</span>.
          </p>
          <p class="m-0 text-[length:var(--text-small)] text-faint">{{ NETWORK_SHELL_CAVEAT }}</p>
        </template>

        <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,220px),1fr))] gap-4">
          <FormField v-for="field in spec.fields" :key="field.field" :label="field.label" :hint="fieldHint(spec.section, field)">
            <template v-if="tagOf(spec.section, field.field)" #tag>
              <StatusTag :data-test="`source-${spec.section}-${field.field}`" v-bind="tagOf(spec.section, field.field)!" />
            </template>
            <template #default="{ id, describedBy }">
              <BaseToggle
                v-if="field.control === 'toggle'"
                :id="id"
                :described-by="describedBy"
                :model-value="toggleValue(spec.section, field.field)"
                :disabled="lockedOn(spec.section, field.field) || lockedOff(spec.section, field.field)"
                :label="field.label"
                hide-label
                @update:model-value="set(spec.section, field.field, $event)"
              />
              <div v-else-if="field.control === 'list'" class="flex flex-col gap-1.5">
                <p
                  v-if="layered"
                  :data-test="`inherited-${spec.section}-${field.field}`"
                  class="m-0 break-words font-mono text-[length:var(--text-micro)] text-faint"
                >
                  Global: {{ asList(base(spec.section, field.field)).join(", ") || "none" }}
                </p>
                <ListField
                  :id="id"
                  :described-by="describedBy"
                  :model-value="asList(own(spec.section, field.field))"
                  :placeholder="layered ? undefined : field.placeholder"
                  @update:model-value="set(spec.section, field.field, $event)"
                />
                <p
                  v-if="redundant(spec.section, field.field).length"
                  :data-test="`redundant-${spec.section}-${field.field}`"
                  class="m-0 text-[length:var(--text-small)] text-state-running"
                >
                  Already in the global policy: <span class="font-mono">{{ redundant(spec.section, field.field).join(", ") }}</span>
                </p>
              </div>
              <input
                v-else
                :id="id"
                :aria-describedby="describedBy"
                :value="numberValue(spec.section, field.field)"
                :placeholder="numberPlaceholder(spec.section, field.field)"
                type="number"
                min="0"
                step="1"
                class="form-control font-mono tabular-nums"
                @input="setNumber(spec.section, field.field, ($event.target as HTMLInputElement).value)"
              />
            </template>
          </FormField>

          <FormField
            v-if="spec.section === 'terminal'"
            label="Session idle timeout"
            :hint="sessionIdle > 0 ? 'Seconds of inactivity before a terminal session closes.' : 'Sessions stay open until reset.'"
          >
            <template v-if="tagOf('terminal', SESSION_IDLE)" #tag>
              <StatusTag :data-test="`source-terminal-${SESSION_IDLE}`" v-bind="tagOf('terminal', SESSION_IDLE)!" />
            </template>
            <template #default="{ id, describedBy }">
              <div class="flex items-center gap-3">
                <BaseToggle :model-value="sessionIdle > 0" label="Close idle sessions" hide-label @update:model-value="setSessionIdle" />
                <input
                  v-if="sessionIdle > 0"
                  :id="id"
                  :aria-describedby="describedBy"
                  :value="sessionIdle"
                  type="number"
                  min="1"
                  step="1"
                  class="form-control w-28 font-mono tabular-nums"
                  @input="setNumber('terminal', SESSION_IDLE, ($event.target as HTMLInputElement).value)"
                />
              </div>
            </template>
          </FormField>
        </div>

        <Callout v-if="spec.section === 'terminal' && externalPaths.length" tone="warning" title="External file system access">
          The agent can reach paths outside its workspace directory:
          <span class="font-mono text-primary">{{ externalPaths.join(", ") }}</span>. This weakens isolation — keep it for trusted work only.
        </Callout>
      </template>
    </section>
  </div>
</template>
