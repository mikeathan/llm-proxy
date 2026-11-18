<script setup lang="ts">
import { computed } from "vue"
import SelectInput from "../common/forms/SelectInput.vue"
import { RADIO_KIND_ANY, RADIO_KIND_NOT_KNOWN, RADIO_KIND_PICK_LABEL, RADIO_KIND_WORDS, radioKindOption } from "../../constants/radioIdentity"
import type { RadioKindCount, RadioKindFilter } from "../../types/radio"

// Radio · the kind drop-down beside the chips: what a device is (router, phone, speaker…), with how many of each the chips allow. It narrows on
// top of the chip. A kind with none is not offered, except the one picked, so the choice never vanishes under you.
const props = defineProps<{ kinds: RadioKindCount[]; modelValue: RadioKindFilter }>()
const emit = defineEmits<{ (e: "update:modelValue", value: RadioKindFilter): void }>()
const total = computed(() => props.kinds.reduce((n, k) => n + k.count, 0))
const options = computed(() => {
  const list = props.kinds.map((k) => ({ value: k.kind as string, label: radioKindOption(k.kind === "unknown" ? RADIO_KIND_NOT_KNOWN : RADIO_KIND_WORDS[k.kind], k.count) }))
  if (props.modelValue !== "any" && !props.kinds.some((k) => k.kind === props.modelValue)) {
    list.push({ value: props.modelValue, label: radioKindOption(props.modelValue === "unknown" ? RADIO_KIND_NOT_KNOWN : RADIO_KIND_WORDS[props.modelValue], 0) })
  }
  return [{ value: "any", label: radioKindOption(RADIO_KIND_ANY, total.value) }, ...list]
})
</script>

<template>
  <label class="flex items-center gap-2 text-[length:var(--text-small)] text-secondary" data-test="radio-kind-pick">{{ RADIO_KIND_PICK_LABEL }}
    <SelectInput :model-value="modelValue" :options="options" :label="RADIO_KIND_PICK_LABEL" class="max-sm:!h-11" data-test="radio-kind-select" @update:model-value="emit('update:modelValue', $event as RadioKindFilter)" />
  </label>
</template>
