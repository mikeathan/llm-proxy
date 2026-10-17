<script setup lang="ts">
import { computed, toRef } from "vue"
import { useProfiles } from "../../../composables/assistant/useProfiles"
import { useWorkspaceProfile } from "../../../composables/assistant/useWorkspaceProfile"
import { defaultProfile } from "../../../utils/assistant/profileChoice"
import FormField from "../../common/forms/FormField.vue"
import SelectInput from "../../common/forms/SelectInput.vue"

// The profile this workspace's new chats start with (how the assistant behaves here). Saves on change, on its own:
// the rest of the workspace's settings keep their own Save. A chat can still pick another profile for itself.
const props = defineProps<{ workspaceId: string }>()

const { profiles, error: catalogError } = useProfiles()
const { profile, loading, saving, error, save } = useWorkspaceProfile(toRef(props, "workspaceId"))

const DEFAULT_VALUE = ""
const fallback = computed(() => defaultProfile(profiles.value, "assistant"))
const options = computed(() => [
  { value: DEFAULT_VALUE, label: fallback.value ? `Default (${fallback.value.name})` : "Default" },
  ...profiles.value.map((p) => ({ value: p.id, label: p.name })),
])
// A choice that no longer exists (a deleted profile file) reads as the default, which is what the server runs.
const shown = computed(() => (profiles.value.some((p) => p.id === profile.value) ? profile.value : DEFAULT_VALUE))
const summary = computed(() => (defaultProfile(profiles.value, "assistant", shown.value) ?? fallback.value)?.summary ?? "")
const problem = computed(() => error.value ?? catalogError.value)
</script>

<template>
  <FormField label="Profile for new chats" :hint="summary">
    <template #default="{ id }">
      <SelectInput
        :id="id"
        :model-value="shown"
        :options="options"
        label="Profile for new chats"
        :disabled="loading || saving || profiles.length === 0"
        :aria-busy="loading || saving"
        @update:model-value="save($event)"
      />
    </template>
  </FormField>
  <p v-if="problem" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ problem }}</p>
</template>
