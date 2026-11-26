<script setup lang="ts">
import { computed, ref, watch } from "vue";
import ContextDrawer from "../layout/ContextDrawer.vue";
import FormField from "../common/forms/FormField.vue";
import ModelEditorForm from "./ModelEditorForm.vue";
import { useConfirm } from "../../composables/ui/useConfirm";
import type { APIKeyItem, ProviderType } from "../../types/admin";
import type { AvailableModel, Model, ModelEditorOpen } from "../../types/model";

// The model editor as a side panel over the Models page (ADR-0090, option A of the mocks): the list stays in view, Escape
// or the scrim close it, and closing with unsaved edits asks first. Adding starts with where the model runs: this computer or a provider.

const props = defineProps<{
  open: ModelEditorOpen | null;
  models: Model[];
  availableModels: AvailableModel[];
  /** The cloud providers, in order, with their names. */
  providers: readonly { id: ProviderType; label: string }[];
  apiKeysOf: (provider: ProviderType) => APIKeyItem[];
}>();

const emit = defineEmits<{
  (e: "update:open", open: ModelEditorOpen | null): void;
  (e: "refresh"): void;
}>();

const LOCAL: ProviderType = "local";
const LOCAL_LABEL = "This computer (llama.cpp)";
const DISCARD_PROMPT = {
  title: "Discard your changes?",
  message: "The model keeps its saved settings.",
  type: "warning",
  confirmText: "Discard",
  cancelText: "Keep editing",
} as const;

const { confirm } = useConfirm();
const dirty = ref(false);
watch(() => props.open, () => (dirty.value = false));

const provider = computed<ProviderType>(() => {
  const open = props.open;
  if (!open) return LOCAL;
  if (open.kind === "edit") return open.model.provider;
  if (open.kind === "file") return LOCAL;
  return open.provider;
});
const title = computed(() => {
  const open = props.open;
  if (!open) return "";
  if (open.kind === "edit") return `Edit ${open.model.name}`;
  if (open.kind === "file") return `Add ${open.file.metadata?.name || open.file.name}`;
  return "Add a model";
});
// The form is made again for each model or provider it opens on, so no draft carries over.
const formKey = computed(() => JSON.stringify(props.open && { kind: props.open.kind, provider: provider.value, name: props.open.kind === "edit" ? props.open.model.name : "" }));

async function close() {
  if (dirty.value && !(await confirm(DISCARD_PROMPT))) return;
  emit("update:open", null);
}

const chooseProvider = (id: string) => emit("update:open", { kind: "add", provider: id as ProviderType });
</script>

<template>
  <!-- Teleported to <body>, as other overlays are: inside the page it sat under the app header and the phone's tab bar, which hid its
       title, Close and Save on a phone. -->
  <Teleport to="body">
  <ContextDrawer :open="!!open" :title="title" wide @update:open="(value: boolean) => !value && close()">
    <template v-if="open">
      <FormField v-if="open.kind === 'add'" label="Where it runs" class="mb-5">
        <template #default="{ id }">
          <select :id="id" :value="provider" class="form-control" @change="chooseProvider(($event.target as HTMLSelectElement).value)">
            <option :value="LOCAL">{{ LOCAL_LABEL }}</option>
            <option v-for="p in providers" :key="p.id" :value="p.id">{{ p.label }}</option>
          </select>
        </template>
      </FormField>
      <ModelEditorForm
        :key="formKey"
        :provider="provider"
        :api-keys="apiKeysOf(provider)"
        :models="models"
        :available-models="availableModels"
        :open="open"
        @refresh="emit('refresh')"
        @dirty="dirty = $event"
        @close="emit('update:open', null)"
      />
    </template>
  </ContextDrawer>
  </Teleport>
</template>
