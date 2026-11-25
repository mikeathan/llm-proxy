<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import DashboardAddBoard from "./DashboardAddBoard.vue"
import { useDismissable } from "../../composables/ui/useDismissable"
import { ADD_CARD_LABEL, EDITING_BOARD_LABEL, PRESET_OPTIONS, RESET_BOARD_LABEL } from "../../constants/dashboard"
import type { DashboardBoardTemplate, DashboardPreset } from "../../types/dashboard"

// The bar shown while editing the dashboard (Edit in the page head), under the tabs: everything that changes the board waits here, so reading
// shows only the title, Edit and the tabs. The layout is a choice that re-flows every card of this board only; once a card is moved or resized
// by hand it reads "Custom". A phone shows every card full width whatever the layout, so the layout menu is left out below sm. The card list is
// the slot.
const props = defineProps<{ preset: DashboardPreset | "custom"; canReset: boolean; boardName: string; offered: DashboardBoardTemplate[]; atLimit: boolean }>()
const emit = defineEmits<{
  (e: "preset", value: DashboardPreset): void
  (e: "reset"): void
  (e: "add-board", template?: DashboardBoardTemplate): void
  (e: "reset-all"): void
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
  <div ref="root" class="relative flex flex-col gap-2 rounded-[var(--radius-md)] border border-dashed border-accent-brand p-2.5" data-test="dashboard-toolbar">
    <p class="m-0 font-mono text-[11px] font-medium uppercase tracking-[var(--tracking-micro)] text-accent-brand">{{ EDITING_BOARD_LABEL(boardName) }}</p>
    <div class="flex flex-wrap items-center gap-2">
      <label class="flex items-center gap-2 text-[length:var(--text-small)] text-secondary max-sm:hidden">Layout
        <SelectInput :model-value="props.preset" :options="options" label="Layout of this board" data-test="layout-select" @update:model-value="choose" />
      </label>
      <BaseButton variant="secondary" size="sm" icon="plus" :aria-expanded="open" aria-controls="card-catalogue" class-name="max-sm:!h-11" data-test="add-card" @click="open = !open">{{ ADD_CARD_LABEL }}</BaseButton>
      <DashboardAddBoard :offered="offered" :at-limit="atLimit" @add="emit('add-board', $event)" @reset-all="emit('reset-all')" />
      <BaseButton v-if="canReset" variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="reset-board" @click="emit('reset')">{{ RESET_BOARD_LABEL }}</BaseButton>
    </div>
    <slot v-if="open" name="catalogue" :close="() => (open = false)" />
  </div>
</template>
