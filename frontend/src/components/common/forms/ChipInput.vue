<script setup lang="ts">
import { ref } from "vue"

// A list typed as chips: type a value and press Enter (or a comma) to add it, press Backspace in the empty box to take the last one back, or use a chip's
// cross. Values are trimmed, and a repeat (in any letter case) is not added twice. `id` / `describedBy` come from the surrounding FormField.
const props = defineProps<{ modelValue: readonly string[]; label: string; id?: string; describedBy?: string; placeholder?: string; lower?: boolean }>()
const emit = defineEmits<{ (e: "update:modelValue", value: string[]): void }>()

const draft = ref("")
function commit() {
  const v = props.lower ? draft.value.trim().toLowerCase() : draft.value.trim()
  draft.value = ""
  if (v === "" || props.modelValue.some((x) => x.toLowerCase() === v.toLowerCase())) return
  emit("update:modelValue", [...props.modelValue, v])
}
const remove = (i: number) => emit("update:modelValue", props.modelValue.filter((_, j) => j !== i))
function onKey(e: KeyboardEvent) {
  if (e.key === "Enter" || e.key === ",") {
    e.preventDefault()
    commit()
  } else if (e.key === "Backspace" && draft.value === "" && props.modelValue.length) remove(props.modelValue.length - 1)
}
</script>

<template>
  <div class="form-control flex min-h-10 flex-wrap items-center gap-1.5 py-1.5" data-test="chip-input">
    <span v-for="(c, i) in modelValue" :key="c" class="inline-flex max-w-full items-center gap-1 rounded-full bg-surface-active py-0.5 pl-2.5 pr-1 text-[length:var(--text-small)] text-primary">
      <span class="truncate">{{ c }}</span>
      <button type="button" class="inline-flex h-5 w-5 items-center justify-center rounded-full text-muted hover:text-primary" :aria-label="`Remove ${c}`" @click="remove(i)">×</button>
    </span>
    <input
      :id="id"
      v-model="draft"
      type="text"
      :aria-label="label"
      :aria-describedby="describedBy"
      :placeholder="modelValue.length ? '' : placeholder"
      class="min-w-[8rem] flex-1 border-0 bg-transparent p-0 text-primary outline-none"
      @keydown="onKey"
      @blur="commit"
    />
  </div>
</template>
