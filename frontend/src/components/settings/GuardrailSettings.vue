<script setup lang="ts">
import type { AgentGuardrailsConfig, GlobalConfig } from "../../types/admin";
import GuardrailForm from "../AgentIde/system/GuardrailForm.vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import Panel from "../common/layout/Panel.vue";

// Settings · Guardrails: the system-wide policy every workspace inherits. The
// page saves it with the rest of the configuration; resetting only changes the
// draft (distinct from saving), so Discard still brings the old policy back.
const props = defineProps<{
  config: GlobalConfig;
}>();

const emit = defineEmits<{
  (e: "update:config", config: GlobalConfig): void;
}>();

function handleFormUpdate(guardrails: AgentGuardrailsConfig) {
  emit("update:config", { ...props.config, guardrails });
}

// An empty policy makes the backend fall back to the manifest defaults
// (GetGuardrails merges them in), so the next load shows the defaults.
function resetToDefaults() {
  emit("update:config", { ...props.config, guardrails: {} as AgentGuardrailsConfig });
}
</script>

<template>
  <Panel title="System-wide guardrails">
    <template #actions>
      <BaseButton variant="ghost" size="sm" icon="refresh" @click="resetToDefaults">Reset to defaults</BaseButton>
    </template>
    <p class="mb-4 mt-0 max-w-[72ch] text-[length:var(--text-small)] text-muted">
      The policy every workspace inherits. A workspace can add to it under its own Settings.
    </p>
    <GuardrailForm :model-value="config.guardrails" @update:model-value="handleFormUpdate" />
  </Panel>
</template>
