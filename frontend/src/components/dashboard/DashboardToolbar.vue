<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import { useDismissable } from "../../composables/ui/useDismissable"
import { ADD_CARD_LABEL, ARRANGE_DONE_LABEL, ARRANGE_LABEL, PRESET_OPTIONS, RESET_BOARD_LABEL } from "../../constants/dashboard"
import type { DashboardPreset } from "../../types/dashboard"

// The controls of the open board: its layout, arranging, adding a card, and putting a shipped board back. The layout is a choice that
// re-flows every card of this board only; once a card is moved or resized by hand it reads "Custom". The card list itself is the slot.
const props = defineProps<{ preset: DashboardPreset | "custom"; arrange: boolean; canReset: boolean }>()
const emit = defineEmits<{
  (e: "preset", value: DashboardPreset): void
  (e: "arrange", on: boolean): void
  (e: "reset"): void
}>()
defineSlots<{ catalogue(props: { close: () => void }): unknown }>()

const options = [...PRESET_OPTIONS, { value: "custom", label: "Custom" }]
const open = ref(false)
const root = ref<HTMLElement | null>(null)
useDismissable(root, open)
const choose = (value: string) => {
  if (value !== "custom") emit("preset", value as DashboardPreset)
}
</script>

<template>
  <div ref="root" class="relative flex flex-wrap items-center gap-2" data-test="dashboard-toolbar">
    <label class="flex items-center gap-2 text-[length:var(--text-small)] text-secondary">Layout
      <SelectInput :model-value="props.preset" :options="options" label="Layout of this board" class="max-sm:!h-11" data-test="layout-select" @update:model-value="choose" />
    </label>
    <BaseButton variant="secondary" size="sm" :aria-pressed="arrange" class-name="max-sm:!h-11" data-test="arrange" @click="emit('arrange', !arrange)">{{ arrange ? ARRANGE_DONE_LABEL : ARRANGE_LABEL }}</BaseButton>
    <BaseButton variant="secondary" size="sm" icon="plus" :aria-expanded="open" aria-controls="card-catalogue" class-name="max-sm:!h-11" data-test="add-card" @click="open = !open">{{ ADD_CARD_LABEL }}</BaseButton>
    <BaseButton v-if="canReset" variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="reset-board" @click="emit('reset')">{{ RESET_BOARD_LABEL }}</BaseButton>
    <slot v-if="open" name="catalogue" :close="() => (open = false)" />
  </div>
</template>
