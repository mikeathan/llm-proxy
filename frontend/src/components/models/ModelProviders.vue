<script setup lang="ts">
import { reactive } from "vue";
import Disclosure from "../common/layout/Disclosure.vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import FormField from "../common/forms/FormField.vue";
import ApiKeySettings from "../settings/ApiKeySettings.vue";
import type { APIKeyItem, GlobalConfig, ProviderKeyTest, ProviderTestStatus, ProviderType } from "../../types/admin";
import type { Model } from "../../types/model";

// Models › Providers: one folding section per cloud provider with its API keys (saved at once, masked by the server), Add a model, and
// for Gemini the Vertex AI fields of the shared configuration. A provider with a key opens; one with none stays a quiet line.

const props = defineProps<{
  providers: readonly { id: ProviderType; label: string }[];
  keys: Partial<Record<ProviderType, APIKeyItem[]>>;
  testStatus: Partial<Record<ProviderType, ProviderTestStatus>>;
  models: Model[];
  editConfig: GlobalConfig;
  showBaseUrl: (provider: ProviderType) => boolean;
}>();

const emit = defineEmits<{
  (e: "save-keys", provider: ProviderType, keys: APIKeyItem[]): void;
  (e: "test-key", provider: ProviderType, payload: ProviderKeyTest): void;
  (e: "clear-test", provider: ProviderType): void;
  (e: "clear-keys", provider: ProviderType): void;
  (e: "add-model", provider: ProviderType): void;
  (e: "remove-models", provider: ProviderType): void;
  (e: "update:editConfig", config: GlobalConfig): void;
}>();

const GEMINI: ProviderType = "gemini";

const keysOf = (p: ProviderType) => props.keys[p] ?? [];
const modelsOf = (p: ProviderType) => props.models.filter((m) => m.provider === p);
function modelCountsByKey(p: ProviderType): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const m of modelsOf(p)) {
    const key = m.provider_config?.api_key_name || "";
    counts[key] = (counts[key] || 0) + 1;
  }
  return counts;
}
const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? "" : "s"}`;
const summary = (p: ProviderType) => (keysOf(p).length ? `${plural(keysOf(p).length, "key")} · ${plural(modelsOf(p).length, "model")}` : "Not set up");

// Each section's fold; a provider with a key starts open.
const open = reactive<Partial<Record<ProviderType, boolean>>>({});
const isOpen = (p: ProviderType) => open[p] ?? keysOf(p).length > 0;

const vertex = (field: "project_id" | "region") => props.editConfig.providers?.[GEMINI]?.[field] ?? "";
const setVertex = (field: "project_id" | "region", value: string) => {
  const gemini = props.editConfig.providers?.[GEMINI] ?? { type: GEMINI, api_keys: [] };
  emit("update:editConfig", { ...props.editConfig, providers: { ...props.editConfig.providers, [GEMINI]: { ...gemini, [field]: value } } });
};
</script>

<template>
  <div class="flex flex-col gap-2">
    <Disclosure
      v-for="p in providers"
      :key="p.id"
      :open="isOpen(p.id)"
      :title="p.label"
      :summary="summary(p.id)"
      :data-provider="p.id"
      class="border border-hairline"
      @update:open="open[p.id] = $event"
    >
      <template #aside>
        <BaseButton variant="secondary" size="sm" icon="plus" :disabled="!keysOf(p.id).length" @click="emit('add-model', p.id)">Add a model</BaseButton>
      </template>
      <ApiKeySettings
        :api-keys="keysOf(p.id)"
        :show-base-url="showBaseUrl(p.id)"
        :test-loading="!!testStatus[p.id]?.loading"
        :test-success="testStatus[p.id]?.success"
        :test-error="testStatus[p.id]?.error"
        :model-counts="modelCountsByKey(p.id)"
        @update:api-keys="emit('save-keys', p.id, $event)"
        @test-key="emit('test-key', p.id, $event)"
        @clear-test="emit('clear-test', p.id)"
        @clear-all="emit('clear-keys', p.id)"
      />
      <div v-if="p.id === GEMINI" class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4">
        <FormField label="Vertex AI project ID" hint="Optional. Required only for Vertex AI.">
          <template #default="{ id, describedBy }">
            <input :id="id" :value="vertex('project_id')" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" @input="setVertex('project_id', ($event.target as HTMLInputElement).value)" />
          </template>
        </FormField>
        <FormField label="Vertex AI region" hint="Optional. The Vertex AI region, such as us-central1.">
          <template #default="{ id, describedBy }">
            <input :id="id" :value="vertex('region')" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" @input="setVertex('region', ($event.target as HTMLInputElement).value)" />
          </template>
        </FormField>
      </div>
      <div v-if="modelsOf(p.id).length" class="flex justify-end">
        <BaseButton variant="ghost" size="sm" icon="trash" @click="emit('remove-models', p.id)">Remove all {{ p.label }} models</BaseButton>
      </div>
    </Disclosure>
  </div>
</template>
