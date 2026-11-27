<script setup lang="ts">
import { ref } from "vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import FormField from "../common/forms/FormField.vue";

// The fields of a new API key: value, an optional name and (for providers that take one) a base URL. Inline on a desktop, inside a drawer
// on a phone; the parent adds the key.
defineProps<{ showBaseUrl?: boolean; baseUrlPlaceholder?: string }>();

const emit = defineEmits<{
  (e: "add", draft: { name: string; key: string; base_url?: string }): void;
  (e: "cancel"): void;
}>();

const name = ref("");
const value = ref("");
const baseUrl = ref("");

function add() {
  const key = value.value.trim();
  if (!key) return;
  emit("add", { name: name.value.trim(), key, base_url: baseUrl.value.trim() || undefined });
}
</script>

<template>
  <!-- A plain container, not a <form>: browsers offer to save a password when a form with a password field is submitted. -->
  <div class="flex flex-col gap-3">
    <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,220px),1fr))] gap-4">
      <FormField label="Key value" hint="Stored encrypted; only its last four characters are shown afterwards.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="value" :aria-describedby="describedBy" type="password" class="form-control font-mono" autocomplete="new-password" spellcheck="false" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
        </template>
      </FormField>
      <FormField label="Key name" hint="Optional, e.g. Personal.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="name" :aria-describedby="describedBy" type="text" class="form-control" autocomplete="off" />
        </template>
      </FormField>
      <FormField v-if="showBaseUrl" label="Base URL" hint="Optional.">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="baseUrl" :aria-describedby="describedBy" type="text" class="form-control font-mono" :placeholder="baseUrlPlaceholder" autocomplete="off" />
        </template>
      </FormField>
    </div>
    <div class="flex justify-end gap-2">
      <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" @click="emit('cancel')">Cancel</BaseButton>
      <BaseButton variant="primary" size="sm" icon="plus" class-name="max-sm:!h-11" :disabled="!value.trim()" @click="add">Add key</BaseButton>
    </div>
  </div>
</template>
