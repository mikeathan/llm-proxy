<script setup lang="ts">
import { computed } from "vue"
import type { ChoiceOption } from "../../types/ui"
import type { MemoryMode } from "../../types/automation"
import type { ProfileField, ProfileInput } from "../../types/profile"
import CheckboxGroup from "../common/forms/CheckboxGroup.vue"
import FormField from "../common/forms/FormField.vue"
import ToggleField from "../common/forms/ToggleField.vue"

// One setting of an agent, drawn from the server's description of it (kind, help, choices). The agent itself is not
// changed here: a change is reported with update.
const NO_HELPERS = "There are no other agents to hand tasks to yet."
const props = defineProps<{
  field: ProfileField
  draft: ProfileInput
  error?: string
  disabled?: boolean
  /** The agents this one may hand tasks to (every other agent). */
  helperOptions: ChoiceOption[]
}>()
const emit = defineEmits<{ (e: "update", key: keyof ProfileInput, value: ProfileInput[keyof ProfileInput]): void }>()

const text = computed(() => String(props.draft[props.field.key] ?? ""))
const flag = computed(() => props.draft[props.field.key] === true)
const list = computed(() => (props.draft[props.field.key] as string[] | undefined) ?? [])
const choices = computed<ChoiceOption[]>(() => (props.field.options ?? []).map((o) => ({ value: o.value, label: o.label })))

function set(value: ProfileInput[keyof ProfileInput]) {
  emit("update", props.field.key, value)
}
function onNumber(event: Event) {
  const parsed = Number.parseInt((event.target as HTMLInputElement).value, 10)
  set(Number.isNaN(parsed) ? 0 : parsed)
}
const onChoice = (event: Event) => set((event.target as HTMLSelectElement).value as MemoryMode)
</script>

<template>
  <ToggleField v-if="field.kind === 'toggle'" :model-value="flag" :label="field.label" :hint="error || field.help" :disabled="disabled" @update:model-value="set" />
  <FormField v-else :label="field.label" :hint="field.help" :error="error">
    <template #default="{ id, describedBy, invalid }">
      <textarea
        v-if="field.kind === 'longtext'"
        :id="id"
        :value="text"
        :disabled="disabled"
        :aria-describedby="describedBy"
        :aria-invalid="invalid || undefined"
        rows="14"
        spellcheck="false"
        class="form-control min-h-48 resize-y font-mono leading-relaxed"
        @input="set(($event.target as HTMLTextAreaElement).value)"
      ></textarea>
      <input
        v-else-if="field.kind === 'number'"
        :id="id"
        :value="text"
        :disabled="disabled"
        :aria-describedby="describedBy"
        :aria-invalid="invalid || undefined"
        type="number"
        inputmode="numeric"
        min="0"
        :max="field.max"
        class="form-control w-28 font-mono"
        @input="onNumber"
      />
      <select
        v-else-if="field.kind === 'choice'"
        :id="id"
        :value="text"
        :disabled="disabled"
        :aria-describedby="describedBy"
        class="form-control"
        @change="onChoice"
      >
        <option v-for="option in choices" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
      <CheckboxGroup
        v-else-if="field.kind === 'choices'"
        :model-value="list"
        :options="choices"
        :label="field.label"
        :described-by="describedBy"
        :disabled="disabled"
        @update:model-value="set"
      />
      <CheckboxGroup
        v-else-if="field.kind === 'profiles'"
        :model-value="list"
        :options="helperOptions"
        :label="field.label"
        :described-by="describedBy"
        :disabled="disabled"
        :empty="NO_HELPERS"
        @update:model-value="set"
      />
      <input
        v-else
        :id="id"
        :value="text"
        :disabled="disabled"
        :aria-describedby="describedBy"
        :aria-invalid="invalid || undefined"
        type="text"
        autocomplete="off"
        class="form-control"
        @input="set(($event.target as HTMLInputElement).value)"
      />
    </template>
  </FormField>
</template>
