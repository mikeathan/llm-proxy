<script setup lang="ts">
import { nextTick, ref } from "vue"
import type { ChoiceOption } from "../../../types/ui"

// One-of-n choice drawn as joined segments: a radio group, so only the checked
// segment is in the tab order and the arrow keys move (and wrap).
const props = defineProps<{ modelValue: string; options: ChoiceOption[]; label: string }>()
const emit = defineEmits<{ (e: "update:modelValue", value: string): void }>()

const buttons = ref<HTMLButtonElement[]>([])
const STEP: Record<string, number> = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }

async function onKeydown(event: KeyboardEvent, index: number) {
  const step = STEP[event.key]
  if (!step) return
  event.preventDefault()
  const next = (index + step + props.options.length) % props.options.length
  emit("update:modelValue", props.options[next]!.value)
  await nextTick()
  buttons.value[next]?.focus()
}
</script>

<template>
  <div role="radiogroup" :aria-label="label" class="inline-flex flex-wrap overflow-hidden rounded-[var(--radius-md)] border border-control">
    <button
      v-for="(option, index) in options"
      :key="option.value"
      ref="buttons"
      type="button"
      role="radio"
      :aria-checked="option.value === modelValue"
      :tabindex="option.value === modelValue ? 0 : -1"
      :class="[
        'h-7 px-3 font-mono text-[length:var(--text-small)] font-medium transition-colors duration-fast focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset',
        index > 0 ? 'border-l border-hairline' : '',
        option.value === modelValue ? 'bg-surface-active text-primary' : 'text-muted hover:bg-surface-hover hover:text-primary',
      ]"
      @click="emit('update:modelValue', option.value)"
      @keydown="onKeydown($event, index)"
    >{{ option.label }}</button>
  </div>
</template>
