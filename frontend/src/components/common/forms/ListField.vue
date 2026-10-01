<script setup lang="ts">
import { ref, watch } from "vue"

// A list edited as one entry per line (Phase 5 primitive). Every keystroke
// reports the parsed list — trimmed, blanks dropped — while the textarea keeps
// exactly what was typed; it is only rewritten when the list changes from
// outside. `id` / `describedBy` come from the surrounding FormField.
const props = withDefaults(
  defineProps<{ modelValue: readonly string[]; id?: string; describedBy?: string; rows?: number; placeholder?: string; invalid?: boolean }>(),
  { rows: 3 },
)
const emit = defineEmits<{ (e: "update:modelValue", value: string[]): void }>()

const parse = (text: string) => text.split("\n").map((line) => line.trim()).filter(Boolean)
const same = (a: readonly string[], b: readonly string[]) => a.length === b.length && a.every((item, i) => item === b[i])

const text = ref(props.modelValue.join("\n"))
watch(
  () => props.modelValue,
  (list) => {
    if (!same(parse(text.value), list)) text.value = list.join("\n")
  },
)

function onInput(event: Event) {
  text.value = (event.target as HTMLTextAreaElement).value
  emit("update:modelValue", parse(text.value))
}
</script>

<template>
  <textarea
    :id="id"
    :value="text"
    :rows="rows"
    :placeholder="placeholder"
    :aria-describedby="describedBy"
    :aria-invalid="invalid || undefined"
    spellcheck="false"
    class="form-control font-mono"
    @input="onInput"
  ></textarea>
</template>
