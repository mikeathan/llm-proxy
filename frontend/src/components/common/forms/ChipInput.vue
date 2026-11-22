<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from "vue"
import type { ChipSuggestion } from "../../../types/ui"

// A list typed as chips: type a value and press Enter (or a comma) to add it, press Backspace in the empty box to take the last one back, or use a chip's
// cross. Values are trimmed, and a repeat (in any letter case) is not added twice. `id` / `describedBy` come from the surrounding FormField.
//
// Optional help: `suggestions` are listed under the box while the owner types (matching their label, value or note; Arrow keys and Enter, or a click,
// pick one), `labels` show a chip by a friendlier name than its value, and `accept` says whether a typed value may become a chip: one it refuses stays in
// the box with `rejected` words, and Enter picks the first suggestion instead when there is one. Without them the box works as it always did.
const SUGGESTION_LIMIT = 8
const props = defineProps<{
  modelValue: readonly string[]
  label: string
  id?: string
  describedBy?: string
  placeholder?: string
  lower?: boolean
  suggestions?: readonly ChipSuggestion[]
  labels?: Readonly<Record<string, string>>
  accept?: (value: string) => boolean
  rejected?: string
}>()
const emit = defineEmits<{ (e: "update:modelValue", value: string[]): void }>()

const listId = useId()
const draft = ref("")
const active = ref(-1)
const refused = ref(false)
const has = (v: string): boolean => props.modelValue.some((x) => x.toLowerCase() === v.toLowerCase())
const matches = computed<ChipSuggestion[]>(() => {
  const q = draft.value.trim().toLowerCase()
  if (!props.suggestions || q === "") return []
  return props.suggestions.filter((s) => !has(s.value) && [s.label, s.value, s.note ?? ""].some((t) => t.toLowerCase().includes(q))).slice(0, SUGGESTION_LIMIT)
})
const open = computed(() => matches.value.length > 0)
// On a phone the fixed bottom bar can sit over the list, so when it opens it is scrolled clear (see its scroll margin).
const listEl = ref<HTMLElement | null>(null)
watch(open, async (now) => {
  if (!now) return
  await nextTick()
  listEl.value?.scrollIntoView?.({ block: "nearest" })
})

function add(v: string) {
  draft.value = ""
  active.value = -1
  refused.value = false
  if (v !== "" && !has(v)) emit("update:modelValue", [...props.modelValue, v])
}
function commit() {
  const v = props.lower ? draft.value.trim().toLowerCase() : draft.value.trim()
  if (v !== "" && props.accept && !props.accept(v)) {
    refused.value = true
    return
  }
  add(v)
}
const remove = (i: number) => emit("update:modelValue", props.modelValue.filter((_, j) => j !== i))
function onKey(e: KeyboardEvent) {
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    if (!open.value) return
    e.preventDefault()
    const n = matches.value.length
    // Positions run from "none chosen" (-1) through the last suggestion and wrap round.
    active.value = ((active.value + 1 + (e.key === "ArrowDown" ? 1 : -1) + n + 1) % (n + 1)) - 1
  } else if (e.key === "Escape" && open.value) {
    e.preventDefault()
    draft.value = ""
    active.value = -1
  } else if (e.key === "Enter" || e.key === ",") {
    e.preventDefault()
    const typed = draft.value.trim()
    const pick = matches.value[active.value] ?? (typed !== "" && props.accept && !props.accept(typed) ? matches.value[0] : undefined)
    if (pick) add(pick.value)
    else commit()
  } else if (e.key === "Backspace" && draft.value === "" && props.modelValue.length) remove(props.modelValue.length - 1)
}
function onInput() {
  active.value = -1
  refused.value = false
}
// A value that does not pass `accept` is left in the box when the owner leaves it, so a half-typed word never becomes a chip.
const onBlur = () => commit()
</script>

<template>
  <div class="relative min-w-0">
    <div class="form-control flex min-h-8 flex-wrap items-center gap-1.5 py-[3px]" data-test="chip-input">
      <span v-for="(c, i) in modelValue" :key="c" class="inline-flex max-w-full items-center gap-1 rounded-full bg-surface-active py-0.5 pl-2.5 pr-1 text-[length:var(--text-small)] text-primary">
        <span class="truncate" :title="labels?.[c] ? c : undefined">{{ labels?.[c] ?? c }}</span>
        <button type="button" class="inline-flex h-5 w-5 items-center justify-center rounded-full text-muted hover:text-primary" :aria-label="`Remove ${labels?.[c] ?? c}`" @click="remove(i)">×</button>
      </span>
      <input
        :id="id"
        v-model="draft"
        type="text"
        :role="suggestions ? 'combobox' : undefined"
        :aria-autocomplete="suggestions ? 'list' : undefined"
        :aria-expanded="suggestions ? open : undefined"
        :aria-controls="suggestions ? listId : undefined"
        :aria-activedescendant="active >= 0 ? `${listId}-${active}` : undefined"
        :aria-label="label"
        :aria-describedby="describedBy"
        :aria-invalid="refused || undefined"
        :placeholder="modelValue.length ? '' : placeholder"
        class="min-w-[8rem] flex-1 border-0 bg-transparent p-0 text-primary outline-none"
        @keydown="onKey"
        @input="onInput"
        @blur="onBlur"
      />
    </div>
    <ul
      v-if="suggestions && open"
      :id="listId"
      ref="listEl"
      role="listbox"
      :aria-label="`Suggestions for ${label}`"
      class="absolute left-0 right-0 top-full z-20 mt-1 flex max-h-64 scroll-mb-24 list-none flex-col overflow-auto rounded-[var(--radius-md)] border border-control bg-surface-raised p-1 shadow-lg"
      data-test="chip-suggestions"
    >
      <!-- mousedown (not click) so the box does not blur, and commit its half-typed text, before the pick lands. -->
      <li
        v-for="(s, i) in matches"
        :id="`${listId}-${i}`"
        :key="s.value"
        role="option"
        :aria-selected="i === active"
        :class="['flex cursor-pointer flex-wrap items-baseline gap-x-2 rounded-[var(--radius-sm,4px)] px-2 py-1 text-[length:var(--text-small)] text-primary', i === active ? 'bg-surface-active' : 'hover:bg-surface-active']"
        @mousedown.prevent="add(s.value)"
      >
        <span class="font-medium">{{ s.label }}</span>
        <span v-if="s.note" class="min-w-0 truncate text-faint">{{ s.note }}</span>
      </li>
    </ul>
    <p v-if="refused && rejected" role="alert" class="m-0 mt-1 text-[length:var(--text-small)] text-state-error">{{ rejected }}</p>
  </div>
</template>
