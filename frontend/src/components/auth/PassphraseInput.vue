<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"

// A passphrase field with a Show toggle. `autocomplete` tells a password manager whether this is the current passphrase
// or a new one to save.
const model = defineModel<string>({ required: true })
defineProps<{ label: string; autocomplete: "current-password" | "new-password"; hint?: string; error?: string }>()
const visible = ref(false)
</script>

<template>
  <FormField :label="label" :hint="hint" :error="error">
    <template #default="{ id, describedBy, invalid }">
      <div class="flex min-w-0 items-stretch gap-2">
        <input
          :id="id"
          v-model="model"
          :type="visible ? 'text' : 'password'"
          :autocomplete="autocomplete"
          :aria-describedby="describedBy"
          :aria-invalid="invalid"
          spellcheck="false"
          autocapitalize="none"
          class="min-w-0 flex-1 rounded-[var(--radius-md)] border border-control bg-canvas px-2 py-2 font-mono text-primary focus-visible:outline-none focus-visible:ring-2"
        />
        <BaseButton variant="secondary" :aria-pressed="visible" class-name="max-sm:!h-11" @click="visible = !visible">{{ visible ? "Hide" : "Show" }}</BaseButton>
      </div>
    </template>
  </FormField>
</template>
