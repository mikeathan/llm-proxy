<script setup lang="ts">
import { toRaw } from "vue";
import type { GlobalConfig } from "../../types/admin";
import GuardrailForm from "../AgentIde/system/GuardrailForm.vue";
import BaseButton from "../common/buttons/BaseButton.vue";

const props = defineProps<{
  config: GlobalConfig;
}>();

const emit = defineEmits<{
  (e: "update:config", config: GlobalConfig): void;
  (e: "save"): void;
}>();

function handleFormUpdate(newGuardrails: any) {
  emit("update:config", { ...toRaw(props.config), guardrails: newGuardrails });
}

function handleSave() {
  emit("save");
}

function handleReset() {
  // Clearing the guardrails property in the local config will cause the backend
  // to return manifest defaults (since we use GetGuardrails with merging).
  // However, to show it immediately in UI, we can emit an empty object.
  emit("update:config", { ...toRaw(props.config), guardrails: {} as any });
}
</script>

<template>
  <div class="guardrails-container">
    <div class="header-row">
      <h2 class="settings-title">System-Wide Guardrails</h2>
      <div class="flex gap-2">
        <BaseButton @click="handleReset" variant="ghost" icon="refresh">
          Reset to Defaults
        </BaseButton>
        <BaseButton @click="handleSave" variant="primary" icon="play">
          Save Global Policy
        </BaseButton>
      </div>
    </div>

    <GuardrailForm
      :modelValue="config.guardrails"
      @update:modelValue="handleFormUpdate"
    />
  </div>
</template>

<style scoped lang="postcss">
.guardrails-container {
  @apply space-y-6 animate-in slide-in-from-right-4 duration-300;
}

.header-row {
  @apply flex items-center justify-between mb-2;
}

.settings-title {
  @apply text-xl font-bold text-white;
}

.btn-save {
  @apply bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded-md font-bold text-sm transition-all shadow-lg active:scale-95;
}
</style>
