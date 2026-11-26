<script setup lang="ts">
import { computed } from "vue";
import Panel from "../common/layout/Panel.vue";
import FormField from "../common/forms/FormField.vue";
import SegmentedControl from "../common/forms/SegmentedControl.vue";
import BaseToggle from "../common/buttons/BaseToggle.vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import ToggleField from "../common/forms/ToggleField.vue";
import StorageReportCard from "./StorageReportCard.vue";
import { LOG_LEVELS } from "../../constants/api";
import { useConfigDraftFields } from "../../composables/settings/useConfigDraftFields";
import type { GlobalConfig, MemoryHotDefaults, SettingsTab } from "../../types/admin";
import type { GeneralPanel } from "../../types/admin";
import { GENERAL_PANEL_SECTION } from "../../constants/settings";
import { SHIPPED_MEMORY_DEFAULTS } from "../../constants/memory";
import { DELIVER_TO_HINT, TIMEZONE_HINT, UNKNOWN_TIMEZONE_ERROR, WEB_UI_LABEL } from "../../constants/reminders";
import { defaultConnectorTypes, deliveryLabel, enabledConnectorTypes, withConnectorSkipped } from "../../utils/reminders/channels";
import { browserTimeZone, isKnownTimeZone, knownTimeZones } from "../../utils/format/timezone";
import type { ChoiceOption } from "../../types/ui";

// Settings · General: the global configuration as numbered panels. Every
// field writes a copy of the config through `update:editConfig` (the page owns
// it and its save); only the log level applies on the spot. The model settings
// (routing, the local engine, the run lanes) are on the Models page.

const props = defineProps<{
  /** Which General section is open: only its panels show (all stay mounted). */
  section: SettingsTab;
  editConfig: GlobalConfig;
  logLevel: string;
}>();

const show = (panel: GeneralPanel) => GENERAL_PANEL_SECTION[panel] === props.section;

const emit = defineEmits<{
  (e: "update:editConfig", config: GlobalConfig): void;
  (e: "updateConfig"): void;
  (e: "updateLogLevel", level: string): void;
}>();

const LOG_LEVEL_OPTIONS: ChoiceOption[] = LOG_LEVELS.map((level) => ({ value: level, label: level }));

const { replaceConfig, configField } = useConfigDraftFields(() => props.editConfig, (next) => emit("update:editConfig", next));

const workspacesDir = configField("workspaces_dir");

/** A global hot-memory default as a v-model target; the pair is always written whole. */
function memoryField(key: keyof MemoryHotDefaults) {
  return computed<boolean>({
    get: () => props.editConfig.memory?.[key] ?? SHIPPED_MEMORY_DEFAULTS[key],
    set: (value) => replaceConfig({ memory: { ...SHIPPED_MEMORY_DEFAULTS, ...props.editConfig.memory, [key]: value } }),
  });
}
const assistantHot = memoryField("assistant_hot");
const automationHot = memoryField("automation_hot");

/** The timezone as a v-model target; the reminders section is written whole. */
const reminderTimezone = computed<string>({
  get: () => props.editConfig.reminders?.timezone ?? "",
  set: (value) => replaceConfig({ reminders: { ...props.editConfig.reminders, timezone: value } }),
});
// Only a connector that is switched on can carry a reminder; it does until it is ticked off.
const reminderConnectors = computed(() => enabledConnectorTypes(props.editConfig.communication?.connectors));
const receivesReminders = (type: string) => !(props.editConfig.reminders?.skip_connectors ?? []).includes(type);
const setReceivesReminders = (type: string, receives: boolean) =>
  replaceConfig({ reminders: { ...props.editConfig.reminders, skip_connectors: withConnectorSkipped(props.editConfig.reminders?.skip_connectors, type, receives) } });
const reminderDelivery = computed(() => deliveryLabel(defaultConnectorTypes(reminderConnectors.value, props.editConfig.reminders?.skip_connectors)));
const timezoneError = computed(() => (isKnownTimeZone(reminderTimezone.value) ? undefined : UNKNOWN_TIMEZONE_ERROR));
const TIMEZONE_LIST_ID = "reminder-timezones";
const timezoneChoices = knownTimeZones();

const runLoggingEnabled = computed({
  get: () => props.editConfig.run_logging?.enabled ?? false,
  set: (enabled: boolean) => replaceConfig({ run_logging: { ...props.editConfig.run_logging, enabled } }),
});
</script>

<template>
  <form class="flex flex-col gap-4" @submit.prevent="$emit('updateConfig')">
    <Panel v-show="show('storage')" title="Workspace storage">
      <div class="form-grid">
        <FormField
          label="Workspaces directory"
          hint="Empty: workspaces/ in the repository during development, otherwise in the data root (LLM_PROXY_HOME / --data). Relative paths resolve against the data root."
        >
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="workspacesDir" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" />
          </template>
        </FormField>
        <FormField
          label="Run logging"
          hint="Each run gets a folder with its event stream and execution history, plus recording.jsonl: every request sent to the model and every reply, with credential-shaped values masked. Runs are kept 30 days. Read a recording at /admin/api/recordings."
        >
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="runLoggingEnabled" :described-by="describedBy" label="Record run logs and model requests" />
          </template>
        </FormField>
      </div>
    </Panel>

    <StorageReportCard v-if="show('storage')" />

    <Panel v-show="show('memory')" title="Memory">
      <p class="mb-4 mt-0 max-w-[64ch] text-[length:var(--text-small)] text-muted">
        Hot memory puts the facts you marked "always" in front of the model at the start of a run. It uses a small
        share of the context window, and only when such facts exist. These are the defaults: a workspace's assistant
        and each automation can override them.
      </p>
      <div class="form-grid">
        <FormField label="Assistant" hint="Chats in every workspace unless the workspace overrides it.">
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="assistantHot" :described-by="describedBy" label="Assistant remembers" />
          </template>
        </FormField>
        <FormField label="Automations" hint="Scheduled and manual runs. Off by default: each run spends part of its context on it.">
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="automationHot" :described-by="describedBy" label="Automations remember" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel v-show="show('reminders')" title="Reminders">
      <p class="mb-4 mt-0 max-w-[64ch] text-[length:var(--text-small)] text-muted">
        Ask the assistant to remind you of something and it is delivered word for word at the time you gave, with no
        model involved.
      </p>
      <div class="form-grid">
        <FormField label="Deliver reminders to" :hint="DELIVER_TO_HINT">
          <template #default>
            <div class="flex flex-col gap-1" data-test="reminder-delivery">
              <ToggleField :model-value="true" disabled :label="WEB_UI_LABEL" hint="Always: every reminder is listed on the Reminders page." />
              <ToggleField
                v-for="type in reminderConnectors"
                :key="type"
                :model-value="receivesReminders(type)"
                :label="type"
                @update:model-value="setReceivesReminders(type, $event)"
              />
              <p v-if="!reminderConnectors.length" class="m-0 text-[length:var(--text-small)] text-faint">No connector is switched on, so reminders stay in the web UI.</p>
              <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="reminder-delivery-summary">Reminders go to: {{ reminderDelivery }}</p>
            </div>
          </template>
        </FormField>
        <FormField label="Timezone" :hint="TIMEZONE_HINT" :error="timezoneError">
          <template #default="{ id, describedBy, invalid }">
            <div class="flex flex-wrap items-center gap-2">
              <input :id="id" v-model.trim="reminderTimezone" :aria-describedby="describedBy" :aria-invalid="invalid" :list="TIMEZONE_LIST_ID" type="text" placeholder="Europe/London" autocomplete="off" class="form-control font-mono" />
              <BaseButton variant="secondary" size="sm" @click="reminderTimezone = browserTimeZone()">Use this browser's timezone</BaseButton>
            </div>
            <datalist :id="TIMEZONE_LIST_ID">
              <option v-for="zone in timezoneChoices" :key="zone" :value="zone"></option>
            </datalist>
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel v-show="show('logging')" title="Logging">
      <FormField label="System log level" hint="Applies immediately — no save needed.">
        <template #default>
          <SegmentedControl :model-value="logLevel" :options="LOG_LEVEL_OPTIONS" label="System log level" class="w-fit" @update:model-value="$emit('updateLogLevel', $event)" />
        </template>
      </FormField>
    </Panel>
  </form>
</template>

<style scoped lang="postcss">
.form-grid {
  @apply grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4;
}
</style>
