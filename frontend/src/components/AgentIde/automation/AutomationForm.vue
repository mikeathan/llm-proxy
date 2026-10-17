<script setup lang="ts">
import { computed, onMounted, ref, toRef, watch } from "vue";
import type { Automation } from "../../../types/dispatcher";
import type { AutomationPayload, MemoryMode } from "../../../types/automation";
import type { AutomationStart } from "../../../types/routes";
import { useAutomationForm } from "../../../composables/automation/useAutomationForm";
import { useHostNetworkState } from "../../../composables/settings/useHostNetworkState";
import { useUnsavedChangesGuard } from "../../../composables/ui/useUnsavedChangesGuard";
import { useMemoryDefaults } from "../../../composables/memory/useMemoryDefaults";
import { taskFileName, useTaskAuthoring } from "../../../composables/automation/useTaskAuthoring";
import { useToast } from "../../../composables/useToast";
import UnsavedTag from "../../common/display/UnsavedTag.vue";
import { loopStrategyDescription } from "../../../utils/model/modelUtils";
import { triggerLabel } from "../../../utils/automation/automationDisplay";
import { memoryModeLabel } from "../../../utils/automation/memoryMode";
import { defaultProfile, profileLabel } from "../../../utils/assistant/profileChoice";
import { useProfiles } from "../../../composables/assistant/useProfiles";
import { DEFAULT_DEDUP_DAYS, MAX_DEDUP_DAYS, busyLabel, deliveryLabel, journalLabel, notifyFromForm, parseDedupDays } from "../../../utils/automation/delivery";
import { toSettings } from "../../../router/routes";
import { RESOURCE_NAME_PATTERN, RESOURCE_NAME_RULE } from "../../../constants/validation";
import Panel from "../../common/layout/Panel.vue";
import FormField from "../../common/forms/FormField.vue";
import ToggleField from "../../common/forms/ToggleField.vue";
import InheritField from "../../common/forms/InheritField.vue";
import BaseButton from "../../common/buttons/BaseButton.vue";
import SchedulePicker from "./SchedulePicker.vue";
import TaskField from "./TaskField.vue";

// Create or edit an automation (plan Phase 5): Basics · Model & access ·
// Schedule · Review. Leaving with unsaved changes asks first.

// Host-level network state (sandboxing.network). When the host switch is
// explicitly OFF every grant below resolves to "none" (models.EffectiveScope:
// host off ⇒ none regardless of grant) — the select stays enabled so operators
// can pre-configure a grant for when the host is re-enabled, with an honest
// inline notice. Shown only once the state is known (never guessed).
const { hostNetworkOff, hostNetworkKnown, load: loadHostNetwork } = useHostNetworkState();
onMounted(loadHostNetwork);

const props = withDefaults(
  defineProps<{
    workspaces: { id: string }[];
    workspaceFiles: Record<string, string[]>;
    editAutomation: Automation | null;
    // Every automation, so a new one cannot reuse a name or overwrite another's task file.
    automations?: Pick<Automation, "workspace" | "name" | "task_file">[];
    // Where a new automation starts: a workspace and a playbook its task begins from (from the playbook library).
    start?: AutomationStart;
  }>(),
  { automations: () => [] },
);

const emit = defineEmits<{
  (e: "create-automation", workspace: string, data: AutomationPayload): void;
  (e: "update-automation", workspace: string, oldName: string, data: AutomationPayload): void;
  (e: "fetch-files", workspace: string): void;
  (e: "cancel"): void;
}>();

const {
  selectedWorkspace,
  form,
  selectedProviderKey,
  filteredModels,
  cloudProvidersWithKeys,
  loopStrategyOptions,
  connectorOptions,
  noConnectors,
  wakeNotice,
  handleSubmit: validateSubmit,
  resetForm,
} = useAutomationForm(
  toRef(props, "editAutomation"),
  (ws) => emit("fetch-files", ws),
);

const toast = useToast();
const authoring = useTaskAuthoring();
// Writing a task is a create-only choice. The view keeps one form across new and edit, so the choice must not outlive it.
const writingTask = computed(() => !props.editAutomation && authoring.mode.value === "new");
watch(
  () => props.editAutomation?.id,
  () => authoring.reset(),
);
// The playbook library is only fetched once someone asks to write a new task.
watch(writingTask, (on) => {
  if (on) void authoring.loadPlaybooks();
});
// Opened from a playbook ("Create automation" in the library): workspace chosen, task started from it, named after it.
// Applied once per link, and never when editing.
watch(
  () => `${props.start?.workspace ?? ""}|${props.start?.playbook ?? ""}`,
  () => void applyStart(),
  { immediate: true },
);
async function applyStart() {
  const { workspace, playbook } = props.start ?? {};
  if (!playbook || props.editAutomation) return;
  if (workspace) selectedWorkspace.value = workspace;
  form.value.name ||= playbook;
  authoring.mode.value = "new";
  await authoring.loadPlaybooks();
  await onChoosePlaybook(playbook);
}

const playbookKey = ref(0);
async function onChoosePlaybook(id: string) {
  try {
    if (!(await authoring.choosePlaybook(id))) playbookKey.value++;
  } catch (err) {
    playbookKey.value++;
    toast.error(`Could not load that playbook: ${err instanceof Error ? err.message : err}`);
  }
}
const newTaskFile = computed(() => (form.value.name ? taskFileName(form.value.name) : ""));

const NETWORK_LABEL: Record<string, string> = {
  "": "Inherit workspace settings",
  none: "No network",
  lan: "Local network only",
  internet_only: "Internet only",
  internet: "Local network + Internet",
};

const { automationDefaultOn } = useMemoryDefaults();
const memoryLabel = (mode: MemoryMode) => memoryModeLabel(mode, automationDefaultOn.value);

// The profile says how the run behaves; the hint reads what the chosen one (or the default) is for.
const { profiles } = useProfiles();
// An automation whose saved profile no longer exists keeps that value visible (and replaceable) rather than a blank select.
const profileChoices = computed(() => {
  const saved = form.value.profile;
  const missing = saved && !profiles.value.some((p) => p.id === saved);
  return [...profiles.value.map((p) => ({ value: p.id, label: p.name })), ...(missing ? [{ value: saved, label: `${saved} (not found)` }] : [])];
});
const profileHint = computed(() => (profiles.value.find((p) => p.id === form.value.profile) ?? defaultProfile(profiles.value, "automation"))?.summary);

const loopStrategyHelper = computed(() => {
  if (!form.value.loopStrategy) {
    return "Uses the model's configured loop strategy (react by default).";
  }
  return loopStrategyDescription(form.value.loopStrategy);
});

const DEDUP_DAYS_RULE = `Enter a whole number of days from 1 to ${MAX_DEDUP_DAYS}, or leave it empty for ${DEFAULT_DEDUP_DAYS}.`;
const dedupDaysError = computed(() =>
  form.value.notifyDedup && parseDedupDays(form.value.notifyDedupDays) === null ? DEDUP_DAYS_RULE : "",
);

const taskFiles = computed(() => (selectedWorkspace.value ? props.workspaceFiles[selectedWorkspace.value] ?? [] : []));

// ── Validation ───────────────────────────────────────────────────────────
const nameError = ref("");

// What a new automation would collide with: another automation of that name in the workspace, or (when the task is
// written here) the file it would be saved as being some other automation's task. Checked before anything is written.
const collision = computed(() => {
  if (props.editAutomation || !selectedWorkspace.value || !form.value.name) return "";
  const inWorkspace = props.automations.filter((a) => a.workspace === selectedWorkspace.value);
  if (inWorkspace.some((a) => a.name === form.value.name)) {
    return `An automation named ${form.value.name} already exists in this workspace.`;
  }
  const owner = writingTask.value ? inWorkspace.find((a) => a.task_file === newTaskFile.value) : undefined;
  return owner ? `${newTaskFile.value} is already the task of "${owner.name}". Choose another name.` : "";
});

// The server's reason when it says the chosen schedule is not valid (reported by the picker).
const scheduleError = ref("");

const missing = computed(() => {
  if (!selectedWorkspace.value) return "Choose a workspace.";
  if (!form.value.name) return "Name the automation.";
  if (collision.value) return collision.value;
  if (writingTask.value ? !authoring.content.value.trim() : !form.value.taskFile) {
    return writingTask.value ? "Write the task." : "Choose a task file.";
  }
  if (form.value.triggerType !== "manual" && !form.value.triggerValue) return "Set when it runs.";
  if (scheduleError.value) return "The schedule is not valid.";
  if (dedupDaysError.value) return dedupDaysError.value;
  return "";
});

// ── Unsaved changes ──────────────────────────────────────────────────────
// A snapshot of the form as loaded; a successful submit hands off to the
// owner, so it is not "unsaved" while the owner navigates away.
const snapshot = ref("");
const submitted = ref(false);
const current = computed(() =>
  JSON.stringify({ ws: selectedWorkspace.value, form: form.value, task: [authoring.mode.value, authoring.content.value] }),
);
watch(() => props.editAutomation?.id, () => {
  snapshot.value = current.value;
}, { immediate: true });
watch(current, () => {
  submitted.value = false;
});
const isDirty = computed(() => !submitted.value && current.value !== snapshot.value);
useUnsavedChangesGuard(isDirty);

// Saves the task written in the form as <name>.md and points the automation at it.
// False when nothing should be created: replacing an existing file was declined, or the write failed.
async function saveWrittenTask(): Promise<boolean> {
  try {
    const file = await authoring.saveTask(selectedWorkspace.value, form.value.name);
    if (!file) return false;
    form.value.taskFile = file;
    emit("fetch-files", selectedWorkspace.value);
    return true;
  } catch (err) {
    toast.error(`Could not save the task file: ${err instanceof Error ? err.message : err}`);
    return false;
  }
}

// True while a written task is being saved, so a second press cannot send a second create.
const saving = ref(false);

const handleSubmit = async () => {
  if (missing.value || saving.value) return;
  if (!RESOURCE_NAME_PATTERN.test(form.value.name)) {
    nameError.value = RESOURCE_NAME_RULE;
    return;
  }
  nameError.value = "";
  if (writingTask.value) {
    saving.value = true;
    const saved = await saveWrittenTask().finally(() => (saving.value = false));
    if (!saved) return;
  }
  const data = validateSubmit();
  if (!data) return;

  const payload: AutomationPayload = {
    name: data.name,
    trigger: { type: data.triggerType, value: data.triggerValue },
    task_file: data.taskFile,
    strategy: data.strategy,
    model: data.model,
    loop_strategy: data.loopStrategy,
    network_grant: data.networkGrant,
    memory_mode: data.memoryMode,
    notify: notifyFromForm(data),
    skip_if_busy: data.triggerType !== "manual" && data.skipIfBusy,
    journal: data.journal,
    profile: data.profile,
  };

  submitted.value = true;
  if (props.editAutomation) {
    emit("update-automation", selectedWorkspace.value, props.editAutomation.name, payload);
  } else {
    emit("create-automation", selectedWorkspace.value, payload);
    resetForm();
    authoring.reset();
    snapshot.value = current.value;
  }
};

const review = computed(() => [
  ["Workspace", selectedWorkspace.value || "—"],
  ["Name", form.value.name || "—"],
  ["Task file", writingTask.value ? (newTaskFile.value ? `${newTaskFile.value} (new file)` : "—") : form.value.taskFile || "—"],
  ["Model", form.value.model || "Workspace default"],
  ["Profile", profileLabel(profiles.value, "automation", form.value.profile)],
  ["Runs", triggerLabel({ trigger: form.value.triggerType, trigger_value: form.value.triggerValue })],
  ["Network", NETWORK_LABEL[form.value.networkGrant] ?? form.value.networkGrant],
  ["Memory", memoryLabel(form.value.memoryMode)],
  ["Delivery", deliveryLabel(notifyFromForm(form.value))],
  ["When busy", busyLabel(form.value.triggerType !== "manual" && form.value.skipIfBusy)],
  ["Journal", journalLabel(form.value.journal)],
]);
</script>

<template>
  <form class="flex flex-col gap-4" novalidate @submit.prevent="handleSubmit">
    <Panel title="Basics">
      <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4">
        <FormField label="Workspace" :hint="editAutomation ? 'An automation stays in its workspace.' : undefined">
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="selectedWorkspace" :aria-describedby="describedBy" :disabled="!!editAutomation" class="form-control">
              <option value="" disabled>Choose a workspace…</option>
              <option v-for="ws in workspaces" :key="ws.id" :value="ws.id">{{ ws.id }}</option>
            </select>
          </template>
        </FormField>
        <FormField label="Name" :hint="RESOURCE_NAME_RULE" :error="nameError || collision">
          <template #default="{ id, describedBy, invalid }">
            <input :id="id" v-model="form.name" :aria-describedby="describedBy" :aria-invalid="invalid ? 'true' : undefined" placeholder="e.g. daily-sync" class="form-control font-mono text-[length:var(--text-small)]" />
          </template>
        </FormField>
      </div>
      <TaskField
        class="mt-4"
        :task-file="form.taskFile"
        :files="taskFiles"
        :mode="writingTask ? 'new' : 'existing'"
        :content="authoring.content.value"
        :playbook-id="authoring.playbookId.value"
        :playbooks="authoring.playbooks.value"
        :playbooks-failed="authoring.playbooksFailed.value"
        :workspace-chosen="!!selectedWorkspace"
        :can-choose-mode="!editAutomation"
        :new-file-name="newTaskFile"
        :playbook-key="playbookKey"
        @update:task-file="form.taskFile = $event"
        @update:mode="authoring.mode.value = $event"
        @update:content="authoring.content.value = $event"
        @choose-playbook="onChoosePlaybook"
      />
    </Panel>

    <Panel title="Model & access">
      <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4">
        <FormField label="Connection">
          <template #default="{ id }">
            <select :id="id" v-model="selectedProviderKey" class="form-control">
              <option value="">Workspace default</option>
              <option value="local">Local model</option>
              <optgroup v-for="p in cloudProvidersWithKeys" :key="p.providerName" :label="p.providerName">
                <option v-for="k in p.keys" :key="k.id" :value="`${p.providerName}/${k.keyVal}`">{{ p.providerName }} · {{ k.name }}</option>
              </optgroup>
            </select>
          </template>
        </FormField>
        <FormField label="Model">
          <template #default="{ id }">
            <select :id="id" v-model="form.model" :disabled="!selectedProviderKey" class="form-control font-mono text-[length:var(--text-small)]">
              <option value="">{{ selectedProviderKey ? "Choose a model…" : "Choose a connection first" }}</option>
              <option v-for="m in filteredModels" :key="m.name" :value="m.name">{{ m.name }}</option>
            </select>
          </template>
        </FormField>
        <FormField label="Profile" :hint="profileHint">
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="form.profile" :aria-describedby="describedBy" class="form-control">
              <option value="">{{ profileLabel(profiles, "automation") }}</option>
              <option v-for="p in profileChoices" :key="p.value" :value="p.value">{{ p.label }}</option>
            </select>
          </template>
        </FormField>
        <FormField label="Loop strategy" :hint="loopStrategyHelper">
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="form.loopStrategy" :aria-describedby="describedBy" class="form-control">
              <option value="">Use the model's setting</option>
              <option v-for="opt in loopStrategyOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
            </select>
          </template>
        </FormField>
        <FormField
          label="Network access"
          hint="Overrides this automation's network scope for its runs only; terminal commands keep direct access once any network is allowed."
        >
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="form.networkGrant" :aria-describedby="describedBy" class="form-control">
              <option v-for="(label, value) in NETWORK_LABEL" :key="value" :value="value">{{ label }}</option>
            </select>
          </template>
        </FormField>
        <FormField
          label="Memory"
          hint="Gives each run your workspace's always-on facts, once per run. It costs a small share of the context window. The default is set in Settings."
        >
          <template #default="{ describedBy }">
            <InheritField v-model="form.memoryMode" :default-on="automationDefaultOn" label="Memory" :aria-describedby="describedBy" />
          </template>
        </FormField>
      </div>
      <ToggleField
        v-model="form.journal"
        class="mt-3"
        label="Keep a journal between runs"
        hint="The run reads its own notes from earlier runs and rewrites them before it finishes: queries that surfaced new items, sources to skip, topics already covered. You can read or clear the journal on the automation's page."
      />
      <p v-if="wakeNotice" data-test="wake-notice" role="note" class="m-0 mt-3 text-[length:var(--text-small)] text-state-running">{{ wakeNotice }}</p>
      <p v-if="hostNetworkKnown && hostNetworkOff" role="note" class="m-0 mt-3 text-[length:var(--text-small)] text-state-running">
        The host network is off (Settings → Security &amp; Sandboxing), so runs resolve to "No network" until it is enabled. The grant is kept for then.
      </p>
    </Panel>

    <Panel title="Delivery">
      <div class="flex flex-col gap-3">
        <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4">
          <FormField
            label="Send results to"
            hint="After each run the report goes to this connector. You also get a short message if a run fails."
          >
            <template #default="{ id, describedBy }">
              <select :id="id" v-model="form.notifyConnector" :aria-describedby="describedBy" class="form-control">
                <option value="">Don't send results</option>
                <option v-for="opt in connectorOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
              </select>
            </template>
          </FormField>
        </div>
        <p v-if="noConnectors" role="note" class="m-0 text-[length:var(--text-small)] text-muted">
          No connectors yet.
          <RouterLink :to="toSettings('communication')" class="text-accent-info-text hover:underline">Add one in Settings → Communication</RouterLink>
          to receive results in Telegram.
        </p>
        <template v-if="form.notifyConnector">
          <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,280px),1fr))] gap-x-6 gap-y-1">
            <ToggleField
              v-model="form.notifyDedup"
              label="Skip items already reported"
              hint="Rows whose link you already received are left out, so recurring digests stay fresh."
            />
            <ToggleField
              v-model="form.notifySendEmpty"
              label="Message me when nothing is new"
              hint="Off keeps quiet runs silent. The run still appears in its history."
            />
          </div>
          <FormField
            v-if="form.notifyDedup"
            label="Remember reported items for (days)"
            :hint="`Empty uses ${DEFAULT_DEDUP_DAYS} days.`"
            :error="dedupDaysError"
          >
            <template #default="{ id, describedBy, invalid }">
              <input
                :id="id"
                v-model="form.notifyDedupDays"
                :aria-describedby="describedBy"
                :aria-invalid="invalid ? 'true' : undefined"
                :placeholder="String(DEFAULT_DEDUP_DAYS)"
                inputmode="numeric"
                class="form-control max-w-[12rem] font-mono text-[length:var(--text-small)]"
              />
            </template>
          </FormField>
        </template>
      </div>
    </Panel>

    <Panel title="Schedule">
      <div class="flex flex-col gap-3">
        <SchedulePicker
          :type="form.triggerType"
          :value="form.triggerValue"
          @update:trigger="
            form.triggerType = $event.type;
            form.triggerValue = $event.value;
          "
          @update:error="scheduleError = $event"
        />
        <p v-if="form.triggerType === 'manual'" class="m-0 text-[length:var(--text-small)] text-muted">Runs only when started from this page, the list, or the API.</p>
        <ToggleField
          v-if="form.triggerType !== 'manual'"
          v-model="form.skipIfBusy"
          label="Skip a run if the model is busy"
          hint="If a chat or another run is using the model when this fires, skip it and wait for the next one. Manual runs always wait their turn. The workspace Heartbeat does this on its own."
        />
      </div>
    </Panel>

    <Panel title="Review">
      <dl data-test="review" class="m-0 grid gap-2">
        <div v-for="[term, value] in review" :key="term" class="grid grid-cols-[8rem_minmax(0,1fr)] gap-3">
          <dt class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">{{ term }}</dt>
          <dd class="m-0 break-words text-secondary">{{ value }}</dd>
        </div>
      </dl>
      <div class="mt-4 flex flex-wrap items-center justify-end gap-2">
        <UnsavedTag v-if="isDirty" class="mr-auto" />
        <span v-if="missing" class="text-[length:var(--text-small)] text-muted">{{ missing }}</span>
        <BaseButton variant="secondary" @click="emit('cancel')">Cancel</BaseButton>
        <BaseButton type="submit" :disabled="!!missing">{{ editAutomation ? "Save changes" : "Create automation" }}</BaseButton>
      </div>
    </Panel>
  </form>
</template>
