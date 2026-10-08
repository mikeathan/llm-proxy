<script setup lang="ts">
import { computed } from "vue"
import SegmentedControl from "./SegmentedControl.vue"
import type { ChoiceOption } from "../../../types/ui"

// A per-item override of a global on/off default: "Default (on|off)", "On" or
// "Off". Inherit is the empty string, so the first option always says what
// inheriting means right now. `label` is the radio group's accessible name.
const props = defineProps<{ modelValue: string; defaultOn: boolean; label: string }>()
const emit = defineEmits<{ (e: "update:modelValue", value: string): void }>()

const options = computed<ChoiceOption[]>(() => [
  { value: "", label: `Default (${props.defaultOn ? "on" : "off"})` },
  { value: "on", label: "On" },
  { value: "off", label: "Off" },
])
</script>

<template>
  <SegmentedControl :model-value="modelValue" :options="options" :label="label" class="w-fit" @update:model-value="emit('update:modelValue', $event)" />
</template>
