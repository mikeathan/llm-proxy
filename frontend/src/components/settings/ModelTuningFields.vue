<script setup lang="ts">
import { computed } from "vue";
// ModelTuningFields.vue — the agent-tuning + safety-timeout form block,
// extracted from the 4× duplicated blocks in ProviderModelsCard.vue (D2).
// Field policy is driven by the server-computed workload_class (§2.7): local
// workloads show max_tokens/context_budget as derived (read-only, n_ctx math);
// cloud workloads are editable and prefilled from published capabilities.
// The fields edit the add / edit draft the parent passes in (`model`).
import FormField from "../common/forms/FormField.vue";
import BaseToggle from "../common/buttons/BaseToggle.vue";
import MicroLabel from "../common/display/MicroLabel.vue";
import { useTuningFieldPolicy } from "../../composables/settings/useTuningFieldPolicy";
import { loopStrategyDescription } from "../../utils/model/modelUtils";
import type { LoopStrategyOption, TuningFields, WorkloadClass } from "../../types/model";
import type { ProviderType, ReasoningCapability } from "../../types/admin";

const props = defineProps<{
  model: TuningFields;
  provider: ProviderType;
  workloadClass: WorkloadClass;
  reasoning?: ReasoningCapability;
  loopStrategyOptions?: LoopStrategyOption[];
}>();

const policy = useTuningFieldPolicy(props.workloadClass);
const DERIVED_HINT = "Derived from the model's serving context (n_ctx). Read-only.";

// Mode-aware hint: effort-mode providers express "off" as the provider
// default (omitted reasoning_effort) rather than a hard disable, until
// reasoning_effort:"none" is verified end-to-end.
const reasoningHint = computed(() =>
  props.reasoning?.mode === "effort"
    ? "On sends an explicit reasoning effort; off leaves the provider default."
    : "Turns on the provider's native reasoning for this model.",
);

// The hint under the loop-strategy select describes the selected value
// (empty → ReAct's).
const selectedLoopStrategyDescription = computed(() => loopStrategyDescription(props.model.loop_strategy ?? ""));
const maxTokensHint = computed(() =>
  policy.isLocal ? DERIVED_HINT : "Output cap per response. Values above a published cap are clamped by the backend.",
);
</script>

<template>
  <div class="flex flex-col gap-4">
    <MicroLabel>Agent tuning · per-model overrides</MicroLabel>
    <div class="tuning-grid">
      <FormField label="Max steps" hint="Agent loop iterations before it must finish. Default 25.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model.number="model.max_steps" :aria-describedby="describedBy" type="number" class="form-control font-mono tabular-nums" placeholder="25" min="1" max="100" />
        </template>
      </FormField>
      <FormField label="Context budget (chars)" :hint="policy.isLocal ? DERIVED_HINT : 'History kept before older turns are truncated.'">
        <template #default="{ id, describedBy }">
          <input
            :id="id"
            v-model.number="model.context_budget"
            :aria-describedby="describedBy"
            :readonly="policy.contextBudget === 'derived'"
            type="number"
            class="form-control font-mono tabular-nums read-only:text-muted"
            placeholder="8000"
            min="1000"
            max="100000"
            step="1000"
          />
        </template>
      </FormField>
      <FormField label="Max tokens" :hint="maxTokensHint">
        <template #default="{ id, describedBy }">
          <input
            :id="id"
            v-model.number="model.max_tokens"
            :aria-describedby="describedBy"
            :readonly="policy.maxTokens === 'derived'"
            type="number"
            class="form-control font-mono tabular-nums read-only:text-muted"
            placeholder="3072"
            min="64"
            max="131072"
            step="64"
          />
        </template>
      </FormField>
      <FormField label="Reasoning budget" hint="Thinking tokens before the answer. 0 = computed from max tokens.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model.number="model.reasoning_budget" :aria-describedby="describedBy" type="number" class="form-control font-mono tabular-nums" placeholder="0" min="0" max="65536" step="128" />
        </template>
      </FormField>
      <FormField v-if="policy.isCloud && reasoning?.toggleable" label="Enable thinking" :hint="reasoningHint">
        <template #default="{ id, describedBy }">
          <!-- Unset stays unset until toggled (nullable reasoning_enabled contract). -->
          <BaseToggle :id="id" :model-value="!!model.reasoning_enabled" :described-by="describedBy" label="Provider reasoning" @update:model-value="model.reasoning_enabled = $event" />
        </template>
      </FormField>
      <FormField label="Temperature" hint="0 = deterministic, 2 = creative. Default 0.1.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model.number="model.temperature" :aria-describedby="describedBy" type="number" class="form-control font-mono tabular-nums" placeholder="0.1" min="0" max="2" step="0.05" />
        </template>
      </FormField>
      <FormField label="Timeout (min)" hint="Per run. 0 = the global default (30 min).">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model.number="model.timeout_minutes" :aria-describedby="describedBy" type="number" class="form-control font-mono tabular-nums" placeholder="0" min="0" max="120" step="5" />
        </template>
      </FormField>
      <FormField label="Tool call format" hint="How tool calls are written in the request. Auto: native for cloud, XML text for local GGUF models.">
        <template #default="{ id, describedBy }">
          <select :id="id" v-model="model.tool_call_format" :aria-describedby="describedBy" class="form-control">
            <option value="">Auto (cloud: native · local: XML)</option>
            <option value="native">Native tools</option>
            <option value="xml">XML text</option>
          </select>
        </template>
      </FormField>
      <FormField label="Loop strategy" :hint="selectedLoopStrategyDescription">
        <template #default="{ id, describedBy }">
          <select :id="id" v-model="model.loop_strategy" :aria-describedby="describedBy" class="form-control">
            <option value="">Provider default (ReAct)</option>
            <option v-for="opt in loopStrategyOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
          </select>
        </template>
      </FormField>
      <FormField label="Prefill" hint="Starts each reply with a tool-call stub to steer the output format.">
        <template #default="{ id, describedBy }">
          <BaseToggle :id="id" :model-value="!!model.prefill" :described-by="describedBy" label="Prefill tool calls" @update:model-value="model.prefill = $event" />
        </template>
      </FormField>
    </div>

    <MicroLabel>Safety timeouts · 0 = off</MicroLabel>
    <div class="tuning-grid">
      <FormField label="Per-tool timeout (sec)" hint="One tool call, except filesystem.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model.number="model.tool_timeout_seconds" :aria-describedby="describedBy" type="number" class="form-control font-mono tabular-nums" placeholder="120" min="0" max="600" step="5" />
        </template>
      </FormField>
      <FormField label="Filesystem timeout (sec)" hint="One file read or write.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model.number="model.filesystem_tool_timeout_seconds" :aria-describedby="describedBy" type="number" class="form-control font-mono tabular-nums" placeholder="30" min="0" max="300" step="5" />
        </template>
      </FormField>
      <FormField label="Max plan duration (min)" hint="Wall-clock time for a whole plan.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model.number="model.max_plan_duration_minutes" :aria-describedby="describedBy" type="number" class="form-control font-mono tabular-nums" placeholder="15" min="0" max="120" step="5" />
        </template>
      </FormField>
      <FormField label="Max plan steps" hint="Steps one plan may contain.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model.number="model.max_plan_steps" :aria-describedby="describedBy" type="number" class="form-control font-mono tabular-nums" placeholder="50" min="0" max="500" step="5" />
        </template>
      </FormField>
      <FormField label="Guardrail timeout (sec)" hint="Time allowed for a guardrail check.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model.number="model.guardrail_timeout_seconds" :aria-describedby="describedBy" type="number" class="form-control font-mono tabular-nums" placeholder="5" min="0" max="60" step="1" />
        </template>
      </FormField>
      <FormField label="Guardrail timeout behaviour" hint="What a timed-out check does with the tool call.">
        <template #default="{ id, describedBy }">
          <select :id="id" v-model="model.guardrail_timeout_behavior" :aria-describedby="describedBy" class="form-control">
            <option value="fail-open">Fail open (allow)</option>
            <option value="fail-closed">Fail closed (reject)</option>
          </select>
        </template>
      </FormField>
      <FormField label="Guardrail approval timeout (sec)" hint="How long a blocked call waits for your allow / deny. 0 = the global default (5 min).">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model.number="model.guardrail_approval_timeout_seconds" :aria-describedby="describedBy" type="number" class="form-control font-mono tabular-nums" placeholder="300" min="0" max="3600" step="5" />
        </template>
      </FormField>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.tuning-grid {
  @apply grid grid-cols-[repeat(auto-fit,minmax(min(100%,220px),1fr))] gap-4;
}
</style>
