<script setup lang="ts">
// An on/off switch (Phase 5 primitive): a native checkbox with role="switch",
// so click, Space and form semantics come for free. The label is always the
// accessible name; `hideLabel` keeps it for assistive tech only, when the row
// already shows it. `id` / `describedBy` let a FormField own the label.
import { nextTick } from 'vue'

const props = defineProps<{
  modelValue: boolean
  label: string
  hideLabel?: boolean
  disabled?: boolean
  id?: string
  describedBy?: string
}>()
const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()

// The checkbox flips as soon as it is clicked; the model is the truth. If the parent leaves the model as it was (a change that
// was refused, or a save that failed), the box goes back to match it instead of showing a state that was never kept.
function change(event: Event) {
  const input = event.target as HTMLInputElement
  emit('update:modelValue', input.checked)
  void nextTick(() => { input.checked = props.modelValue })
}
</script>

<template>
  <label
    class="inline-flex w-fit items-center gap-2.5 text-secondary"
    :class="disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'"
  >
    <input
      :id="id"
      type="checkbox"
      role="switch"
      class="peer sr-only"
      :checked="modelValue"
      :disabled="disabled"
      :aria-describedby="describedBy"
      @change="change"
    />
    <span
      aria-hidden="true"
      class="relative h-4 w-[30px] flex-none rounded-[2px] border border-control transition-colors after:absolute after:left-0.5 after:top-0.5 after:h-2.5 after:w-2.5 after:bg-text-muted after:transition-transform after:duration-[var(--motion-fast)] after:content-[''] peer-checked:border-state-success peer-checked:after:translate-x-[14px] peer-checked:after:bg-state-success peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-focus-ring"
    ></span>
    <span :class="hideLabel ? 'sr-only' : 'text-[length:var(--text-small)]'">{{ label }}</span>
  </label>
</template>
