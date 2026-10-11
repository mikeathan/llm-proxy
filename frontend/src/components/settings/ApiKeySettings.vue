<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { APIKeyItem, ProviderKeyTest } from "../../types/admin";
import { useConfirm } from "../../composables/ui/useConfirm";
import BaseButton from "../common/buttons/BaseButton.vue";
import FormField from "../common/forms/FormField.vue";
import MicroLabel from "../common/display/MicroLabel.vue";

// A cloud provider's API keys: pick one to edit, test or remove it, or add a
// new one. The page saves the whole list as soon as it changes (the server
// answers with masked values), so there is no separate Save here. Removals are
// confirmed; a removed key takes the models that use it along.
const props = defineProps<{
  apiKeys: APIKeyItem[];
  testLoading: boolean;
  testSuccess?: string;
  testError?: string;
  showBaseUrl?: boolean;
  modelCounts?: Record<string, number>;
}>();

const emit = defineEmits<{
  (e: "update:apiKeys", keys: APIKeyItem[]): void;
  (e: "testKey", payload: ProviderKeyTest): void;
  (e: "clearTest"): void;
  (e: "clearAll"): void;
}>();

const MASK = "••••••••";
const TAIL = 4;
const { confirm } = useConfirm();

const masked = (key: string) => `${MASK}${key.slice(-TAIL)}`;
const modelsUsing = (name: string) => props.modelCounts?.[name] ?? 0;
const plural = (n: number) => `${n} model${n === 1 ? "" : "s"}`;

// ── Edit the selected key ────────────────────────────────────────────────
const selectedId = ref<string | null>(null);
const editName = ref("");
const editValue = ref("");
const editBaseUrl = ref("");
const showValue = ref(false);
const editPanelId = "api-key-editor";

const selected = computed(() => props.apiKeys.find((k) => k.id === selectedId.value) ?? null);
const canSaveEdit = computed(() => !!editName.value.trim() && !!editValue.value.trim());

// A key removed elsewhere (clear all, a refresh) closes its editor.
watch(
  () => props.apiKeys,
  (keys) => {
    if (selectedId.value && !keys.some((k) => k.id === selectedId.value)) closeEditor();
  },
);

function openEditor(item: APIKeyItem) {
  selectedId.value = item.id;
  editName.value = item.name;
  editValue.value = item.key;
  editBaseUrl.value = item.base_url || "";
  showValue.value = false;
  emit("clearTest");
}

function closeEditor() {
  selectedId.value = null;
  emit("clearTest");
}

const toggleEditor = (item: APIKeyItem) => (selectedId.value === item.id ? closeEditor() : openEditor(item));

function saveEdit() {
  const id = selectedId.value;
  if (!id || !canSaveEdit.value) return;
  emit(
    "update:apiKeys",
    props.apiKeys.map((k) =>
      k.id === id ? { ...k, name: editName.value.trim(), key: editValue.value.trim(), base_url: editBaseUrl.value.trim() || "" } : k,
    ),
  );
}

function testSelected() {
  if (!selectedId.value) return;
  emit("testKey", {
    key: editValue.value,
    name: editName.value,
    id: selectedId.value,
    base_url: editBaseUrl.value.trim() || undefined,
  });
}

async function removeSelected() {
  const item = selected.value;
  if (!item) return;
  const count = modelsUsing(item.name);
  const ok = await confirm({
    title: `Remove the key ${item.name}?`,
    message: count > 0 ? `${plural(count)} use this key and will be removed with it.` : "The key is deleted from the encrypted store.",
    type: count > 0 ? "error" : "warning",
    confirmText: "Remove key",
  });
  if (!ok) return;
  closeEditor();
  emit("update:apiKeys", props.apiKeys.filter((k) => k.id !== item.id));
}

async function removeAll() {
  const ok = await confirm({
    title: `Remove all ${props.apiKeys.length} keys?`,
    message: "Every key of this provider is deleted, with the models that use them.",
    type: "error",
    confirmText: "Remove all",
  });
  if (ok) emit("clearAll");
}

// ── Add a key ────────────────────────────────────────────────────────────
const newKeyName = ref("");
const newKeyValue = ref("");
const newKeyBaseUrl = ref("");

const newId = () =>
  typeof crypto !== "undefined" && crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(36).substring(2, 11);

function addKey() {
  const key = newKeyValue.value.trim();
  if (!key) return;
  const item: APIKeyItem = {
    id: newId(),
    name: newKeyName.value.trim() || `Key ${props.apiKeys.length + 1}`,
    key,
    base_url: newKeyBaseUrl.value.trim() || undefined,
  };
  emit("update:apiKeys", [...props.apiKeys, item]);
  openEditor(item);
  newKeyName.value = "";
  newKeyValue.value = "";
  newKeyBaseUrl.value = "";
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <p class="m-0 max-w-[72ch] text-[length:var(--text-small)] text-muted">
      Keys are stored encrypted and saved as soon as you add, change or remove one. Select a key to test or edit it.
    </p>

    <p v-if="!apiKeys.length" class="m-0 border border-dashed border-control px-3 py-4 text-center text-[length:var(--text-small)] text-muted">
      No API keys yet — add one below.
    </p>
    <ul v-else class="m-0 flex list-none flex-col divide-y divide-hairline border border-hairline p-0">
      <li v-for="item in apiKeys" :key="item.id">
        <button
          type="button"
          :aria-expanded="selectedId === item.id"
          :aria-controls="selectedId === item.id ? editPanelId : undefined"
          class="flex w-full min-w-0 items-center gap-3 px-3 py-2.5 text-left hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 aria-expanded:bg-surface-active aria-expanded:shadow-[inset_2px_0_0_rgb(var(--accent-brand))]"
          @click="toggleEditor(item)"
        >
          <span class="min-w-0 flex-1">
            <span class="block truncate text-primary">{{ item.name }}</span>
            <span class="flex min-w-0 gap-3 font-mono text-[length:var(--text-micro)] text-muted">
              <span>{{ masked(item.key) }}</span>
              <span v-if="item.base_url" class="truncate">{{ item.base_url }}</span>
            </span>
          </span>
          <span v-if="modelsUsing(item.name)" class="flex-none font-mono text-[length:var(--text-micro)] tabular-nums text-muted">{{ plural(modelsUsing(item.name)) }}</span>
        </button>
      </li>
    </ul>

    <section v-if="selected" :id="editPanelId" :aria-label="`Edit key ${selected.name}`" class="flex flex-col gap-4 border border-control p-4">
      <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,220px),1fr))] gap-4">
        <FormField label="Key name">
          <template #default="{ id }">
            <input :id="id" v-model="editName" type="text" class="form-control" autocomplete="off" />
          </template>
        </FormField>
        <FormField label="Key value" hint="Paste a new key to replace it.">
          <template #default="{ id, describedBy }">
            <div class="flex items-center gap-2">
              <input
                :id="id"
                v-model="editValue"
                :aria-describedby="describedBy"
                :type="showValue ? 'text' : 'password'"
                class="form-control flex-1 font-mono"
                autocomplete="new-password"
                spellcheck="false"
                data-1p-ignore
                data-lpignore="true"
              />
              <BaseButton variant="ghost" size="sm" :aria-pressed="showValue" @click="showValue = !showValue">{{ showValue ? "Hide" : "Show" }}</BaseButton>
            </div>
          </template>
        </FormField>
        <FormField v-if="showBaseUrl" label="Base URL" hint="The API endpoint this key belongs to.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="editBaseUrl" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="https://api.openai.com/v1" autocomplete="off" />
          </template>
        </FormField>
      </div>

      <p v-if="testLoading" role="status" class="m-0 text-[length:var(--text-small)] text-muted">Testing the connection…</p>
      <p v-else-if="testSuccess" role="status" class="m-0 border-l-2 border-state-success bg-state-success/[0.08] px-3 py-2 text-[length:var(--text-small)] text-state-success">{{ testSuccess }}</p>
      <p v-else-if="testError" role="alert" class="m-0 break-words border-l-2 border-state-error bg-state-error/[0.08] px-3 py-2 text-[length:var(--text-small)] text-state-error">{{ testError }}</p>

      <div class="flex flex-wrap items-center gap-2">
        <BaseButton variant="secondary" size="sm" icon="search" :loading="testLoading" @click="testSelected">Test connection</BaseButton>
        <span class="flex-1"></span>
        <BaseButton variant="ghost" size="sm" @click="closeEditor">Close</BaseButton>
        <BaseButton variant="danger" size="sm" icon="trash" @click="removeSelected">Remove key</BaseButton>
        <BaseButton variant="primary" size="sm" icon="check" :disabled="!canSaveEdit" @click="saveEdit">Save key</BaseButton>
      </div>
    </section>

    <!-- A plain container, not a <form>: browsers offer to save a password when a form with a password field is submitted. -->
    <div class="flex flex-col gap-3 border border-dashed border-control p-4">
      <div class="flex items-center justify-between gap-2">
        <MicroLabel>Add a key</MicroLabel>
        <BaseButton v-if="apiKeys.length" variant="ghost" size="sm" icon="trash" @click="removeAll">Remove all keys</BaseButton>
      </div>
      <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,220px),1fr))] gap-4">
        <FormField label="New key name" hint="Optional, e.g. Personal.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="newKeyName" :aria-describedby="describedBy" type="text" class="form-control" autocomplete="off" />
          </template>
        </FormField>
        <FormField label="New key value">
          <template #default="{ id }">
            <input :id="id" v-model="newKeyValue" type="password" class="form-control font-mono" autocomplete="new-password" spellcheck="false" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
          </template>
        </FormField>
        <FormField v-if="showBaseUrl" label="New key base URL" hint="Optional.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="newKeyBaseUrl" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="https://api.openai.com/v1" autocomplete="off" />
          </template>
        </FormField>
      </div>
      <div class="flex justify-end">
        <BaseButton variant="secondary" icon="plus" :disabled="!newKeyValue.trim()" @click="addKey">Add key</BaseButton>
      </div>
    </div>
  </div>
</template>
