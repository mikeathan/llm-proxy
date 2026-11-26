<script setup lang="ts">
import { computed } from "vue";
import Panel from "../common/layout/Panel.vue";
import FormField from "../common/forms/FormField.vue";
import BaseToggle from "../common/buttons/BaseToggle.vue";
import { useConfigDraftFields } from "../../composables/settings/useConfigDraftFields";
import { SUBAGENT_DEFAULT_CONCURRENCY, SUBAGENT_MAX_CONCURRENCY } from "../../constants/settings";
import { HELPER_MODEL_UNAVAILABLE_SUFFIX } from "../../constants/labels";
import type { GlobalConfig, SchedulerConfig } from "../../types/admin";
import type { Model } from "../../types/model";

// Models · Routing: which model answers, and how many runs and helpers go at once. The fields edit the shared configuration draft the
// page owns and saves (moved from Settings · General, ADR-0090).

const props = defineProps<{ editConfig: GlobalConfig; models: Model[] }>();
const emit = defineEmits<{ (e: "update:editConfig", config: GlobalConfig): void }>();

const { replaceConfig, configField } = useConfigDraftFields(() => props.editConfig, (next) => emit("update:editConfig", next));

const primaryModel = configField("primary_model");
const fallbackModel = configField("fallback_model");

// The scheduler section is always written whole, seeded with the shipped
// defaults: the backend treats an absent field as its default, so a partial
// object must never be persisted.
const SCHEDULER_DEFAULTS: Required<SchedulerConfig> = {
  local_concurrency: 1,
  cloud_concurrency: 3,
  preempt_automations: true,
  inbound_wait_seconds: 60,
  inbound_wait_by_default: false,
  inbound_max_queued: 32,
  inbound_preempt: false,
};

/** A scheduler field as a v-model target; `normalise` cleans typed numbers. */
function schedulerField<K extends keyof SchedulerConfig>(
  key: K,
  normalise: (value: Required<SchedulerConfig>[K]) => Required<SchedulerConfig>[K] = (value) => value,
) {
  return computed<Required<SchedulerConfig>[K]>({
    // A present field is never undefined, so the fallback only fills gaps.
    get: () => (props.editConfig.scheduler?.[key] ?? SCHEDULER_DEFAULTS[key]) as Required<SchedulerConfig>[K],
    set: (value) => replaceConfig({ scheduler: { ...SCHEDULER_DEFAULTS, ...props.editConfig.scheduler, [key]: normalise(value) } }),
  });
}
const wholeAtLeast = (min: number, fallback: number) => (value: number) => Math.max(min, Math.floor(value) || fallback);

const localConcurrency = schedulerField("local_concurrency", wholeAtLeast(1, 1));
const cloudConcurrency = schedulerField("cloud_concurrency", wholeAtLeast(1, 1));
const preemptAutomations = schedulerField("preempt_automations");
// Inbound admission for external /v1 callers. Seconds: 0 refuses immediately,
// -1 waits indefinitely (still bounded by the queue depth). A caller that asks
// with X-Queue-Wait is capped at this; a caller that doesn't ask is only parked
// when the parking toggle below is on.
const inboundWaitSeconds = schedulerField("inbound_wait_seconds", wholeAtLeast(-1, 0));
const inboundWaitByDefault = schedulerField("inbound_wait_by_default");
const inboundMaxQueued = schedulerField("inbound_max_queued", wholeAtLeast(1, 1));
const inboundPreempt = schedulerField("inbound_preempt");

// The adaptive result cap is on unless the saved config says otherwise; turning it off is saved as an explicit false.
const adaptiveResultCap = computed({
  get: () => props.editConfig.agent?.adaptive_result_cap ?? true,
  set: (adaptive_result_cap: boolean) => replaceConfig({ agent: { ...props.editConfig.agent, adaptive_result_cap } }),
});

// Helpers (delegated sub-agents) run on a cloud model only: a local one would take the GPU from the parent run.
const cloudModels = computed(() => props.models.filter((m) => m.workload_class === "cloud"));
const subagentModel = computed({
  get: () => props.editConfig.agent?.subagent_model ?? "",
  set: (subagent_model: string) => replaceConfig({ agent: { ...props.editConfig.agent, subagent_model } }),
});
// A helper model that was removed from the models (or is not a cloud model) is still what is saved: it shows, flagged,
// so the user sees what is set and can change it, not a blank box.
const subagentModelUnavailable = computed(() => !!subagentModel.value && !cloudModels.value.some((m) => m.name === subagentModel.value));
const subagentConcurrency = computed({
  get: () => props.editConfig.agent?.subagent_concurrency ?? SUBAGENT_DEFAULT_CONCURRENCY,
  set: (subagent_concurrency: number) => replaceConfig({ agent: { ...props.editConfig.agent, subagent_concurrency } }),
});
</script>

<template>
  <div class="flex flex-col gap-4">
    <Panel title="Which model answers">
      <div class="form-grid">
        <FormField label="Primary model" hint="Serves requests that name no model.">
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="primaryModel" :aria-describedby="describedBy" class="form-control">
              <option value="">Auto — first available</option>
              <option v-for="m in models" :key="m.name" :value="m.name">{{ m.name }} ({{ m.provider }})</option>
            </select>
          </template>
        </FormField>
        <FormField label="Fallback model" hint="Takes over when the primary is offline or fails.">
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="fallbackModel" :aria-describedby="describedBy" class="form-control">
              <option value="">None — no fallback</option>
              <option v-for="m in models" :key="m.name" :value="m.name">{{ m.name }} ({{ m.provider }})</option>
            </select>
          </template>
        </FormField>
        <FormField
          label="Helper model"
          hint="A research chat can hand questions to helpers that search the web in parallel and send back a short answer. They run on this cloud model; Off turns helpers off."
        >
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="subagentModel" :aria-describedby="describedBy" class="form-control">
              <option value="">Off — no helpers</option>
              <option v-if="subagentModelUnavailable" :value="subagentModel">{{ subagentModel }}{{ HELPER_MODEL_UNAVAILABLE_SUFFIX }}</option>
              <option v-for="m in cloudModels" :key="m.name" :value="m.name">{{ m.name }} ({{ m.provider }})</option>
            </select>
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel title="How many at once">
      <p class="mb-4 mt-0 max-w-[64ch] text-[length:var(--text-small)] text-muted">
        How many agent runs execute at once per workload class. Local runs share one GPU; cloud runs run in parallel.
      </p>
      <div class="form-grid">
        <FormField label="Local runs at once">
          <template #default="{ id }">
            <input :id="id" v-model.number="localConcurrency" type="number" min="1" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
        <FormField label="Cloud runs at once">
          <template #default="{ id }">
            <input :id="id" v-model.number="cloudConcurrency" type="number" min="1" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
        <FormField label="Helpers at once" hint="Across the app, beside the run lanes. 1 to 10.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model.number="subagentConcurrency" :aria-describedby="describedBy" type="number" :min="1" :max="SUBAGENT_MAX_CONCURRENCY" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
        <FormField label="Chat priority" hint="A chat message pauses a running automation instead of waiting behind it.">
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="preemptAutomations" :described-by="describedBy" label="Chat preempts running automations" />
          </template>
        </FormField>
        <FormField
          label="Tool results"
          hint="A file, a page or search hits keep more of their text when the model's window has room for it, and never less than 3000 characters. Off keeps a fixed 3000 characters for every result. Applies to the next run."
        >
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="adaptiveResultCap" :described-by="describedBy" label="Size tool results to the room in the window" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel title="External requests">
      <p class="mb-4 mt-0 max-w-[64ch] text-[length:var(--text-small)] text-muted">
        An external client (another proxy, a tool) asking for a different local model would otherwise stop the
        running one. It is held or told to retry instead — a run is only interrupted when you serve the request from
        the run indicator, where queued requests appear and can be dismissed.
      </p>
      <div class="form-grid">
        <FormField label="Wait up to (seconds)" hint="0 refuses at once; -1 waits with no limit.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model.number="inboundWaitSeconds" :aria-describedby="describedBy" type="number" min="-1" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
        <FormField label="Queue depth" hint="Requests held at once; more are refused.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model.number="inboundMaxQueued" :aria-describedby="describedBy" type="number" min="1" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
        <FormField label="Requests that don't ask to wait">
          <template #default="{ id }">
            <BaseToggle :id="id" v-model="inboundWaitByDefault" label="Queue them too" />
          </template>
        </FormField>
        <FormField label="Interrupting">
          <template #default="{ id }">
            <BaseToggle :id="id" v-model="inboundPreempt" label="A waiting request may interrupt the running job" />
          </template>
        </FormField>
      </div>
    </Panel>
  </div>
</template>

<style scoped lang="postcss">
.form-grid {
  @apply grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4;
}
</style>
