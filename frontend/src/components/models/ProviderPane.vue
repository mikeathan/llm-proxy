<script setup lang="ts">
import { computed } from "vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import MicroLabel from "../common/display/MicroLabel.vue";
import StatusTag from "../common/display/StatusTag.vue";
import FormField from "../common/forms/FormField.vue";
import ApiKeySettings from "../settings/ApiKeySettings.vue";
import ProviderMenu from "./ProviderMenu.vue";
import { groupModelsByKey } from "../../domain/models";
import type { APIKeyItem, GlobalConfig, ProviderKeyTest, ProviderTestStatus, ProviderType } from "../../types/admin";
import type { Model } from "../../types/model";

// Models › one cloud provider: its keys (saved at once, masked by the server), and under each key the models it serves, each with Edit.
// Add a model sits in the header and under each key (it picks that key); Gemini adds its Vertex AI fields, which edit the shared configuration
// draft (the page's Save row saves them). Removing every model or key is in the ⋯ menu.

const props = defineProps<{
  provider: ProviderType;
  label: string;
  keys: APIKeyItem[];
  testStatus?: ProviderTestStatus;
  models: Model[];
  editConfig: GlobalConfig;
  showBaseUrl: boolean;
  /** The provider's own default endpoint, from its manifest; empty when it has none. */
  baseUrlPlaceholder?: string;
}>();

const emit = defineEmits<{
  (e: "save-keys", keys: APIKeyItem[]): void;
  (e: "test-key", payload: ProviderKeyTest): void;
  (e: "clear-test"): void;
  (e: "add-model", key?: string): void;
  (e: "edit-model", model: Model): void;
  (e: "remove-models"): void;
  (e: "remove-keys"): void;
  (e: "update:editConfig", config: GlobalConfig): void;
}>();

const GEMINI: ProviderType = "gemini";
const NO_KEY_LABEL = "No key";

const groups = computed(() => groupModelsByKey(props.models, props.keys.map((k) => k.name)));
const modelCounts = computed(() => Object.fromEntries(groups.value.map((g) => [g.key, g.models.length])));

const vertex = (field: "project_id" | "region") => props.editConfig.providers?.[GEMINI]?.[field] ?? "";
const setVertex = (field: "project_id" | "region", value: string) => {
  const gemini = props.editConfig.providers?.[GEMINI] ?? { type: GEMINI, api_keys: [] };
  emit("update:editConfig", { ...props.editConfig, providers: { ...props.editConfig.providers, [GEMINI]: { ...gemini, [field]: value } } });
};
</script>

<template>
  <section :data-provider="provider" data-test="provider-pane" class="flex min-w-0 flex-1 flex-col gap-5">
    <header class="flex items-center gap-2">
      <div class="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1">
        <h2 class="m-0 text-[length:var(--text-heading)] font-semibold text-primary">{{ label }}</h2>
        <StatusTag :state="keys.length ? 'success' : 'neutral'" :label="keys.length ? 'Connected' : 'Not set up'" />
      </div>
      <BaseButton variant="secondary" size="sm" icon="plus" class-name="flex-none max-sm:!h-11" :disabled="!keys.length" @click="emit('add-model')">Add a model</BaseButton>
      <ProviderMenu :label="label" :has-models="models.length > 0" :has-keys="keys.length > 0" @remove-models="emit('remove-models')" @remove-keys="emit('remove-keys')" />
    </header>

    <div class="flex flex-col gap-2">
      <MicroLabel>Keys</MicroLabel>
      <ApiKeySettings
        :api-keys="keys"
        :show-base-url="showBaseUrl"
        :base-url-placeholder="baseUrlPlaceholder"
        :test-loading="!!testStatus?.loading"
        :test-success="testStatus?.success"
        :test-error="testStatus?.error"
        :model-counts="modelCounts"
        @update:api-keys="emit('save-keys', $event)"
        @test-key="emit('test-key', $event)"
        @clear-test="emit('clear-test')"
      />
    </div>

    <div v-if="groups.length" class="flex flex-col gap-3">
      <MicroLabel>Models, by key</MicroLabel>
      <div v-for="g in groups" :key="g.key" :data-key-group="g.key" class="border border-hairline">
        <div class="flex items-center gap-2 border-b border-hairline bg-surface-hover px-3 py-1.5">
          <span class="min-w-0 flex-1 truncate font-mono text-[length:var(--text-small)] text-secondary">{{ g.key || NO_KEY_LABEL }}</span>
          <BaseButton v-if="g.key" variant="ghost" size="sm" icon="plus" @click="emit('add-model', g.key)">Add a model</BaseButton>
        </div>
        <p v-if="!g.models.length" class="m-0 px-3 py-2 text-[length:var(--text-small)] text-muted">No model uses this key yet.</p>
        <ul v-else class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
          <li v-for="m in g.models" :key="m.name" class="flex min-h-11 items-center gap-3 px-3 py-1.5">
            <span class="min-w-0 flex-1 truncate text-primary">{{ m.name }}</span>
            <span v-if="m.model_id && m.model_id !== m.name" class="hidden truncate font-mono text-[length:var(--text-micro)] text-muted sm:inline">{{ m.model_id }}</span>
            <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit ${m.name}`" @click="emit('edit-model', m)" />
          </li>
        </ul>
      </div>
    </div>

    <div v-if="provider === GEMINI" class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4">
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
  </section>
</template>
