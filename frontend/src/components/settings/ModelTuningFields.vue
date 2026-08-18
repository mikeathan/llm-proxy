<script setup lang="ts">
import { computed } from "vue";
// ModelTuningFields.vue — the agent-tuning + safety-timeout form block,
// extracted from the 4× duplicated blocks in ProviderModelsCard.vue (D2).
// Field policy is driven by the server-computed workload_class (§2.7): local
// workloads show max_tokens/context_budget as derived (read-only, n_ctx math);
// cloud workloads are editable and prefilled from published capabilities.
import InfoTooltip from "../common/display/InfoTooltip.vue";
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

// Mode-aware tooltip: effort-mode providers express "off" as the provider
// default (omitted reasoning_effort) rather than a hard disable, until
// reasoning_effort:"none" is verified end-to-end.
const reasoningTooltip = computed(() => {
  if (props.reasoning?.mode === "effort") {
    return "Send explicit reasoning effort (on) vs provider default (off)";
  }
  return "Enable provider-native reasoning for this model";
});

// Helper line under the loop-strategy select: shows the copy for the currently
// selected value (empty → react's).
const selectedLoopStrategyDescription = computed(() =>
  loopStrategyDescription(props.model.loop_strategy ?? ""),
);
</script>

<template>
  <div class="form-section-divider">Agent Tuning (per-model overrides)</div>
  <div class="tuning-grid">
    <div class="form-group">
      <label class="form-label">Max Steps
        <InfoTooltip text="Maximum number of agent loop iterations before forced finalization" />
      </label>
      <input
        v-model.number="model.max_steps"
        type="number"
        class="form-input"
        placeholder="25 (default)"
        min="1" max="100"
      />
    </div>
    <div class="form-group">
      <label class="form-label">Context Budget (chars)
        <InfoTooltip
          text="Character limit for conversation history before truncation"
        />
      </label>
      <div class="relative">
        <input
          v-model.number="model.context_budget"
          type="number"
          class="form-input"
          :class="{ 'form-input--readonly': policy.contextBudget === 'derived' }"
          :readonly="policy.contextBudget === 'derived'"
          placeholder="8000"
          min="1000" max="100000" step="1000"
        />
        <span
          v-if="policy.contextBudget === 'derived'"
          class="derived-pill"
        >derived</span>
      </div>
      <p v-if="policy.isLocal" class="form-helper">
        Derived from the model's serving context (n_ctx). Read-only.
      </p>
    </div>
    <div class="form-group">
      <label class="form-label">Max Tokens
        <InfoTooltip
          text="Maximum tokens per LLM response. Local: derived from serving context. Cloud: overrides the published cap (clamped to it)."
        />
      </label>
      <div class="relative">
        <input
          v-model.number="model.max_tokens"
          type="number"
          class="form-input"
          :class="{ 'form-input--readonly': policy.maxTokens === 'derived' }"
          :readonly="policy.maxTokens === 'derived'"
          placeholder="3072"
          min="64" max="131072" step="64"
        />
        <span
          v-if="policy.maxTokens === 'derived'"
          class="derived-pill"
        >derived</span>
      </div>
      <p v-if="policy.isLocal" class="form-helper">
        Output cap is derived from the serving context. Read-only.
      </p>
      <p v-else-if="policy.isCloud" class="form-helper">
        Output cap. Values above a published cap are clamped by the backend.
      </p>
    </div>
    <div class="form-group">
      <label class="form-label">Reasoning Budget
        <InfoTooltip text="Thinking tokens budget before producing a response. 0 = auto-computed from max_tokens" />
      </label>
      <input
        v-model.number="model.reasoning_budget"
        type="number"
        class="form-input"
        placeholder="0 (auto)"
        min="0" max="65536" step="128"
      />
    </div>
    <div v-if="policy.isCloud && reasoning?.toggleable" class="form-group">
      <label class="form-label">Enable Thinking
        <InfoTooltip :text="reasoningTooltip" />
      </label>
      <label class="flex items-center gap-2 cursor-pointer mt-2">
        <input
          type="checkbox"
          v-model="model.reasoning_enabled"
          class="rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-blue-600"
        />
        <span class="text-sm text-gray-300">Provider reasoning</span>
      </label>
    </div>
    <div class="form-group">
      <label class="form-label">Temperature
        <InfoTooltip text="Controls randomness (0=deterministic, 2=creative). Default: 0.1" />
      </label>
      <input
        v-model.number="model.temperature"
        type="number"
        class="form-input"
        placeholder="0.1"
        min="0" max="2" step="0.05"
      />
    </div>
    <div class="form-group">
      <label class="form-label">Timeout (min)
        <InfoTooltip text="Per-execution timeout in minutes. 0 = use global default (30 min)" />
      </label>
      <input
        v-model.number="model.timeout_minutes"
        type="number"
        class="form-input"
        placeholder="0 (default)"
        min="0" max="120" step="5"
      />
    </div>
    <div class="form-group">
      <label class="form-label">Tool Call Format
        <InfoTooltip text="How tool calls are formatted in the LLM request (native=JSON, xml=XML text). Empty = auto: cloud providers default to native, local llama.cpp/GGUF models default to XML text mode." />
      </label>
      <select v-model="model.tool_call_format" class="form-input">
        <option value="">Auto (cloud: native · local: XML)</option>
        <option value="native">Native Tools</option>
        <option value="xml">XML Text</option>
      </select>
    </div>
    <div class="form-group">
      <label class="form-label">Loop Strategy
        <InfoTooltip text="Controls how the agent loops: ReAct (react), plan-first execution, or a self-review (evaluator-optimizer) loop. Empty = provider default (ReAct)." />
      </label>
      <select v-model="model.loop_strategy" class="form-input">
        <option value="">Provider default (ReAct)</option>
        <option v-for="opt in loopStrategyOptions" :key="opt.value" :value="opt.value">
          {{ opt.label }}
        </option>
      </select>
      <p class="form-helper">{{ selectedLoopStrategyDescription }}</p>
    </div>
    <div class="form-group">
      <label class="form-label">Prefill
        <InfoTooltip text="Prepend a tool-call stub to guide the model's output format" />
      </label>
      <label class="flex items-center gap-2 cursor-pointer mt-2">
        <input
          type="checkbox"
          v-model="model.prefill"
          class="rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-blue-600"
        />
        <span class="text-sm text-gray-300">Prefill tool calls</span>
      </label>
    </div>
  </div>
  <div class="form-section-divider">Safety Timeouts</div>
  <div class="tuning-grid">
    <div class="form-group">
      <label class="form-label">Per-Tool Timeout (sec)
        <InfoTooltip text="Maximum duration for a single tool execution (except filesystem). 0 = disabled" />
      </label>
      <input v-model.number="model.tool_timeout_seconds" type="number" class="form-input" placeholder="120" min="0" max="600" step="5" />
    </div>
    <div class="form-group">
      <label class="form-label">Filesystem Timeout (sec)
        <InfoTooltip text="Maximum duration for filesystem reads and writes. 0 = disabled" />
      </label>
      <input v-model.number="model.filesystem_tool_timeout_seconds" type="number" class="form-input" placeholder="30" min="0" max="300" step="5" />
    </div>
    <div class="form-group">
      <label class="form-label">Max Plan Duration (min)
        <InfoTooltip text="Maximum wall-clock time for plan execution. 0 = disabled" />
      </label>
      <input v-model.number="model.max_plan_duration_minutes" type="number" class="form-input" placeholder="15" min="0" max="120" step="5" />
    </div>
    <div class="form-group">
      <label class="form-label">Max Plan Steps
        <InfoTooltip text="Maximum number of steps a plan can contain. 0 = disabled" />
      </label>
      <input v-model.number="model.max_plan_steps" type="number" class="form-input" placeholder="50" min="0" max="500" step="5" />
    </div>
    <div class="form-group">
      <label class="form-label">Guardrail Timeout (sec)
        <InfoTooltip text="Maximum time for guardrail validation. 0 = disabled" />
      </label>
      <input v-model.number="model.guardrail_timeout_seconds" type="number" class="form-input" placeholder="5" min="0" max="60" step="1" />
    </div>
    <div class="form-group">
      <label class="form-label">Guardrail Timeout Behavior
        <InfoTooltip text="Guardrail timeout response: fail-open (allow) or fail-closed (reject) the tool call" />
      </label>
      <select v-model="model.guardrail_timeout_behavior" class="form-input">
        <option value="fail-open">Fail Open (allow)</option>
        <option value="fail-closed">Fail Closed (reject)</option>
      </select>
    </div>
    <div class="form-group">
      <label class="form-label">Guardrail Approval Timeout (sec)
        <InfoTooltip text="How long the agent waits for your allow/deny decision after a guardrail block. 0 = use global default (5 min)" />
      </label>
      <input v-model.number="model.guardrail_approval_timeout_seconds" type="number" class="form-input" placeholder="300" min="0" max="3600" step="5" />
    </div>
  </div>
</template>

<style scoped>
.form-section-divider {
  @apply text-xs font-bold text-blue-400 uppercase tracking-wide border-t border-gray-700 pt-3 mt-2;
}

.tuning-grid {
  @apply grid grid-cols-1 sm:grid-cols-2 gap-3;
}

.form-group {
  @apply space-y-1;
}

.form-label {
  @apply block text-xs font-semibold text-gray-400;
}

.form-helper {
  @apply text-xs text-gray-500;
}

.form-input {
  @apply w-full bg-gray-800 border border-gray-700 rounded-md px-3 py-2 text-sm text-white
         focus:border-blue-600 focus:ring-1 focus:ring-blue-600 outline-none transition-all;
}

select.form-input {
  @apply appearance-none bg-no-repeat pr-10
         bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236b7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22m6%208%204%204%204-4%22%2F%3E%3C%2Fsvg%3E')]
         bg-[position:right_0.5rem_center] bg-[length:1.25rem_1.25rem];
}

.form-input--readonly {
  @apply bg-gray-900/50 text-gray-500 cursor-not-allowed;
}

.derived-pill {
  @apply absolute right-2 top-1/2 -translate-y-1/2 text-[9px] font-black uppercase tracking-wider px-1.5 py-0.5 rounded-[4px] border leading-none bg-indigo-500/10 text-indigo-400 border-indigo-500/20 pointer-events-none;
}
</style>
