<script setup lang="ts">
import { nextTick, ref } from "vue"
import type { ChoiceOption } from "../../../types/ui"

// One-of-n choice drawn as joined segments: a radio group, so only the checked
// segment is in the tab order and the arrow keys move (and wrap).
// `tone="running"` tints the chosen segment amber while something runs, so it is clear at a glance that the page is live;
// an option marked `live` carries a pulsing dot (still while motion is reduced).
// `touch` makes each segment 44 px high on a phone, for a control that is the way around a page; the default stays compact.
const props = defineProps<{ modelValue: string; options: ChoiceOption[]; label: string; touch?: boolean; tone?: 'running' }>()
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
        touch ? 'max-sm:h-11' : '',
        index > 0 ? 'border-l border-hairline' : '',
        option.value === modelValue ? (tone === 'running' ? 'bg-state-running/[0.28] text-primary ring-1 ring-inset ring-state-running' : 'bg-surface-active text-primary') : 'text-muted hover:bg-surface-hover hover:text-primary',
      ]"
      @click="emit('update:modelValue', option.value)"
      @keydown="onKeydown($event, index)"
    >
      <span v-if="option.live" data-test="live-dot" aria-hidden="true" class="mr-1.5 inline-block size-1.5 rounded-full bg-state-running motion-safe:animate-pulse" />{{ option.label }}<span v-if="option.live" class="sr-only"> (running)</span>
    </button>
  </div>
</template>
