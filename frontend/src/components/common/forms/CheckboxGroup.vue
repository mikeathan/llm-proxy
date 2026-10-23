<script setup lang="ts">
import type { ChoiceOption } from "../../../types/ui"

// A group of checkboxes for picking several of a few (Phase 5 primitive). Each row keeps a 44px touch target on
// phones; the value keeps the options' order. `label` names the group for assistive tech, `describedBy` comes from the
// surrounding FormField.
const props = defineProps<{
  modelValue: readonly string[]
  options: readonly ChoiceOption[]
  label: string
  describedBy?: string
  disabled?: boolean
  empty?: string
}>()
const emit = defineEmits<{ (e: "update:modelValue", value: string[]): void }>()

function toggle(value: string, checked: boolean) {
  const chosen = new Set(props.modelValue)
  if (checked) chosen.add(value)
  else chosen.delete(value)
  emit("update:modelValue", props.options.map((o) => o.value).filter((v) => chosen.has(v)))
}
</script>

<template>
  <p v-if="options.length === 0" class="m-0 text-[length:var(--text-small)] text-faint">{{ empty }}</p>
  <div v-else role="group" :aria-label="label" :aria-describedby="describedBy" class="grid gap-x-4 sm:grid-cols-2">
    <label
      v-for="option in options"
      :key="option.value"
      :class="['flex min-h-11 min-w-0 items-center gap-2.5 text-[length:var(--text-small)] text-secondary sm:min-h-8', disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer']"
    >
      <input
        type="checkbox"
        class="h-4 w-4 flex-none accent-[rgb(var(--accent-brand))]"
        :checked="modelValue.includes(option.value)"
        :disabled="disabled"
        @change="toggle(option.value, ($event.target as HTMLInputElement).checked)"
      />
      <span class="min-w-0 break-words">{{ option.label }}</span>
    </label>
  </div>
</template>
