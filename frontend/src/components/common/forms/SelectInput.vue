<script setup lang="ts">
import { computed } from "vue"
import type { ChoiceOption } from "../../../types/ui"

// A labelled native select (native keeps keyboard, mobile pickers and the
// theme's color-scheme for free). Options with a group are listed under that
// heading (native optgroups, which phone pickers show too), in the order the
// groups first appear; options with none come first.
const props = defineProps<{ modelValue: string; options: ChoiceOption[]; label: string }>()
defineEmits<{ (e: "update:modelValue", value: string): void }>()

const loose = computed(() => props.options.filter((o) => !o.group))
const groups = computed(() => {
  const byName = new Map<string, ChoiceOption[]>()
  for (const o of props.options) if (o.group) byName.set(o.group, [...(byName.get(o.group) ?? []), o])
  return [...byName].map(([name, options]) => ({ name, options }))
})
</script>

<template>
  <select
    :value="modelValue"
    :aria-label="label"
    class="h-8 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2"
    @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
  >
    <option v-for="option in loose" :key="option.value" :value="option.value">{{ option.label }}</option>
    <optgroup v-for="g in groups" :key="g.name" :label="g.name">
      <option v-for="option in g.options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </optgroup>
  </select>
</template>
