<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import FormField from "../common/forms/FormField.vue";
import MicroLabel from "../common/display/MicroLabel.vue";
import ModelTuningFields from "../settings/ModelTuningFields.vue";
import { useProviderModels } from "../../composables/settings/useProviderModels";
import type { APIKeyItem, ProviderType } from "../../types/admin";
import type { AvailableModel, Model, ModelEditorOpen } from "../../types/model";

// The model editor's form: what the model is (Basics), how it runs, and its tuning (ModelTuningFields). It opens on a configured model,
// a new model of one provider, or a file found on disk, and saves, adds or removes through the model registry (useProviderModels).
// It closes itself when a save or a removal is done.

const props = defineProps<{
  provider: ProviderType;
  apiKeys: APIKeyItem[];
  models: Model[];
  availableModels?: AvailableModel[];
  open: ModelEditorOpen;
}>();

const emit = defineEmits<{
  (e: "refresh"): void;
  (e: "close"): void;
  (e: "dirty", dirty: boolean): void;
}>();

const LOCAL = "local";

const {
  providerModels,
  isLoadingModels,
  editingModel,
  isAddingNew,
  modelForm,
  filterText,
  filteredProviderModels,
  editingArgsStr,
  discoveredPath,
  isSubmitDisabled,
  agentDefaults,
  addFormWorkload,
  loadModels,
  startAdd,
  scanAndAdd,
  saveNewModel,
  addDiscoveredModel,
  handleEdit,
  saveEdit,
  handleRemove,
  loopStrategyOptions,
} = useProviderModels(props, () => emit("refresh"));

const isLocal = computed(() => props.provider === LOCAL);

onMounted(() => {
  const open = props.open;
  if (open.kind === "edit") handleEdit(open.model);
  else if (open.kind === "file") addDiscoveredModel(open.file);
  else if (open.key) scanAndAdd(open.key);
  else startAdd();
  snapshot.value = draftText();
});

// Unsaved edits: the draft as it was opened, compared with what it is now.
const draftText = () => JSON.stringify(isAddingNew.value ? modelForm.value : editingModel.value);
const snapshot = ref("");
const dirty = computed(() => !!snapshot.value && draftText() !== snapshot.value);
watch(dirty, (value) => emit("dirty", value));

// A save clears the draft (useProviderModels.cancelEdit); the editor is then done.
watch(editingModel, (model) => {
  if (model === null) emit("close");
});

// Remove asks first (useProviderModels); the editor closes once the model is gone, and stays open when the removal was cancelled or failed.
async function remove() {
  const name = editingModel.value?.name;
  if (!name) return;
  await handleRemove(name);
  await nextTick();
  if (!props.models.some((m) => m.name === name)) emit("close");
}

const submit = () => {
  if (isSubmitDisabled.value) return;
  void (isAddingNew.value ? saveNewModel() : saveEdit());
};

</script>

<template>
  <form v-if="editingModel" class="flex min-h-full flex-col gap-5" @submit.prevent="submit">
    <section class="flex flex-col gap-3" aria-label="Basics">
      <MicroLabel>Basics</MicroLabel>
      <div class="field-grid">
        <!-- Add a local model -->
        <template v-if="isAddingNew && isLocal">
          <FormField label="Model filename" hint="A .gguf file in the model directory.">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="modelForm.filename" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="qwen2.5-7b-instruct-q4_k_m.gguf" autocomplete="off" />
            </template>
          </FormField>
          <FormField label="Full path" hint="Where the file is on the server. Filled in when you pick a file from the disk; otherwise looked up in the model directory, subfolders included, when you save.">
            <template #default="{ id, describedBy }">
              <input :id="id" :value="discoveredPath" :aria-describedby="describedBy" type="text" class="form-control font-mono" readonly placeholder="Looked up when you save" />
            </template>
          </FormField>
          <FormField label="Friendly name" hint="Optional. Derived from the filename when empty.">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="modelForm.name" :aria-describedby="describedBy" type="text" class="form-control" autocomplete="off" />
            </template>
          </FormField>
          <FormField label="Port">
            <template #default="{ id }">
              <input :id="id" v-model.number="modelForm.port" type="number" class="form-control font-mono tabular-nums" />
            </template>
          </FormField>
          <FormField label="Custom arguments" hint="Added to the default arguments for this model.">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="modelForm.args" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="--ctx-size 8192 --gpu-layers 32" spellcheck="false" />
            </template>
          </FormField>
        </template>

        <!-- Add a cloud model -->
        <template v-else-if="isAddingNew">
          <FormField label="API key" hint="The credential this model is called with.">
            <template #default="{ id, describedBy }">
              <select :id="id" v-model="modelForm.key" :aria-describedby="describedBy" class="form-control">
                <option value="">Choose a key</option>
                <option v-for="key in apiKeys" :key="key.id" :value="key.name">{{ key.name }}</option>
              </select>
            </template>
          </FormField>
          <FormField label="Model ID" :hint="providerModels.length ? `${filteredProviderModels.length} of ${providerModels.length} models` : 'Type an ID, or scan the endpoint for its models.'">
            <template #default="{ id, describedBy }">
              <div v-if="providerModels.length" class="flex flex-col gap-1.5">
                <div class="flex items-center gap-2">
                  <input v-model="filterText" type="search" aria-label="Filter models" class="form-control flex-1" placeholder="Filter models…" autocomplete="off" />
                  <BaseButton variant="ghost" size="sm" icon="refresh" icon-only label="Reload the model list" :disabled="!modelForm.key" :loading="isLoadingModels" @click="loadModels(modelForm.key)" />
                </div>
                <select :id="id" v-model="modelForm.id" :aria-describedby="describedBy" size="8" class="form-control font-mono">
                  <option v-for="m in filteredProviderModels" :key="m.id" :value="m.id">{{ m.id }}</option>
                </select>
                <p v-if="!filteredProviderModels.length && filterText" class="m-0 text-[length:var(--text-small)] text-muted">No model matches “{{ filterText }}”.</p>
              </div>
              <div v-else class="flex items-center gap-2">
                <input :id="id" v-model="modelForm.id" :aria-describedby="describedBy" type="text" class="form-control flex-1 font-mono" placeholder="gpt-4o" autocomplete="off" />
                <BaseButton variant="secondary" size="sm" icon="search" :disabled="!modelForm.key" :loading="isLoadingModels" @click="loadModels(modelForm.key)">Scan endpoint</BaseButton>
              </div>
            </template>
          </FormField>
          <FormField label="Friendly name" hint="Optional. Derived from the model ID when empty.">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="modelForm.name" :aria-describedby="describedBy" type="text" class="form-control" autocomplete="off" />
            </template>
          </FormField>
        </template>

        <!-- Edit a local model -->
        <template v-else-if="isLocal">
          <FormField label="Name">
            <template #default="{ id }">
              <input :id="id" v-model="editingModel.name" type="text" class="form-control" autocomplete="off" />
            </template>
          </FormField>
          <FormField label="Filename">
            <template #default="{ id }">
              <input :id="id" v-model="editingModel.filename" type="text" class="form-control font-mono" autocomplete="off" />
            </template>
          </FormField>
          <FormField label="Full path" hint="Where the file is on the server. Read-only. It updates when you save a different filename.">
            <template #default="{ id, describedBy }">
              <input :id="id" :value="editingModel.resolved_path ?? ''" :aria-describedby="describedBy" type="text" class="form-control font-mono" readonly placeholder="Not found yet" />
            </template>
          </FormField>
          <FormField label="Port">
            <template #default="{ id }">
              <input :id="id" v-model.number="editingModel.port" type="number" class="form-control font-mono tabular-nums" />
            </template>
          </FormField>
          <FormField label="Custom arguments" hint="Added to the default arguments for this model.">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="editingArgsStr" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="--ctx-size 8192 --gpu-layers 32" spellcheck="false" />
            </template>
          </FormField>
        </template>

        <!-- Edit a cloud model -->
        <template v-else>
          <FormField label="Name">
            <template #default="{ id }">
              <input :id="id" v-model="editingModel.name" type="text" class="form-control" autocomplete="off" />
            </template>
          </FormField>
          <FormField label="Model ID">
            <template #default="{ id }">
              <input :id="id" v-model="editingModel.model_id" type="text" class="form-control font-mono" autocomplete="off" />
            </template>
          </FormField>
          <FormField v-if="editingModel.provider_config" label="API key">
            <template #default="{ id }">
              <select :id="id" v-model="editingModel.provider_config.api_key_name" class="form-control">
                <option value="">No key (use default)</option>
                <option v-for="key in apiKeys" :key="key.id" :value="key.name">{{ key.name }}</option>
              </select>
            </template>
          </FormField>
        </template>
      </div>

    </section>

    <ModelTuningFields
      v-if="isAddingNew"
      :model="modelForm"
      :provider="provider"
      :workload-class="addFormWorkload"
      :reasoning="agentDefaults?.reasoning"
      :loop-strategy-options="loopStrategyOptions"
    />
    <ModelTuningFields
      v-else
      :model="editingModel"
      :provider="provider"
      :workload-class="editingModel.workload_class || (isLocal ? 'local' : 'cloud')"
      :reasoning="agentDefaults?.reasoning"
      :loop-strategy-options="loopStrategyOptions"
    />

    <div class="sticky bottom-0 mt-auto flex flex-wrap items-center gap-2 border-t border-hairline bg-canvas py-3">
      <BaseButton v-if="!isAddingNew" variant="ghost" icon="trash" @click="remove">Remove</BaseButton>
      <span class="flex-1"></span>
      <BaseButton variant="ghost" @click="emit('close')">Cancel</BaseButton>
      <BaseButton type="submit" variant="primary" icon="check" :disabled="isSubmitDisabled">{{ isAddingNew ? "Add model" : "Save model" }}</BaseButton>
    </div>
  </form>
</template>

<style scoped lang="postcss">
.field-grid {
  @apply grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4;
}
</style>
