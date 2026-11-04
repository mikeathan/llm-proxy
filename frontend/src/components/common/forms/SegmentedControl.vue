<script setup lang="ts">
import { nextTick, ref } from "vue"
import type { ChoiceOption } from "../../../types/ui"

// One-of-n choice drawn as joined segments: a radio group, so only the checked
// segment is in the tab order and the arrow keys move (and wrap).
// `tone="running"` tints the chosen segment amber while something runs, so it is clear at a glance that the page is live;
// an option marked `live` carries a pulsing dot (still while motion is reduced).
// `touch` makes each segment 44 px high on a phone, for a control that is the way around a page; the default stays compact.
// `variant="tabs"` draws the same radio group as an underlined row (scrolls sideways on a phone) for the parts of one page;
// `variant="compact"` is the quiet look at 28 px high on every screen, whose tap area is still 44 px: an invisible layer reaches 8 px
// above and below each word, so a control in a page header does not make the header taller.
// `variant="quiet"` as plain words with the chosen one softly filled, no frame, for a setting that sits beside a title.
const props = defineProps<{ modelValue: string; options: ChoiceOption[]; label: string; touch?: boolean; tone?: 'running'; variant?: 'tabs' | 'quiet' | 'compact' }>()
const emit = defineEmits<{ (e: "update:modelValue", value: string): void }>()

// 28 px high, with a layer 8 px above and below that makes the tap area 44 px (see `variant="compact"` above).
const COMPACT_SEGMENT = "relative h-7 rounded-[var(--radius-sm)] px-2 text-[length:var(--text-small)] transition-colors duration-fast before:absolute before:inset-x-0 before:-inset-y-2 before:content-[''] focus-visible:outline-none focus-visible:ring-2"
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
  <div role="radiogroup" :aria-label="label" :class="variant === 'tabs' ? 'flex max-w-full overflow-x-auto border-b border-hairline' : variant === 'quiet' || variant === 'compact' ? 'inline-flex gap-0.5' : 'inline-flex flex-wrap overflow-hidden rounded-[var(--radius-md)] border border-control'">
    <button
      v-for="(option, index) in options"
      :key="option.value"
      ref="buttons"
      type="button"
      role="radio"
      :aria-checked="option.value === modelValue"
      :tabindex="option.value === modelValue ? 0 : -1"
      :class="variant === 'compact' ? [
        COMPACT_SEGMENT,
        option.value === modelValue ? 'bg-surface-active text-primary' : 'text-muted hover:bg-surface-hover hover:text-primary',
      ] : variant === 'quiet' ? [
        'h-7 rounded-[var(--radius-sm)] px-2.5 text-[length:var(--text-small)] transition-colors duration-fast focus-visible:outline-none focus-visible:ring-2',
        touch ? 'max-sm:h-11' : '',
        option.value === modelValue ? 'bg-surface-active text-primary' : 'text-muted hover:bg-surface-hover hover:text-primary',
      ] : variant === 'tabs' ? [
        'h-11 flex-none whitespace-nowrap border-b-2 px-3.5 text-[length:var(--text-body)] transition-colors duration-fast focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset',
        option.value === modelValue ? 'border-accent-brand font-medium text-primary' : 'border-transparent text-muted hover:text-primary',
      ] : [
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
