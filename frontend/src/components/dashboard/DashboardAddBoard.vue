<script setup lang="ts">
import { ref } from "vue"
import PopoverPanel from "../common/layout/PopoverPanel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import { useDismissable } from "../../composables/ui/useDismissable"
import { ADD_BOARD_LABEL, BOARD_ALREADY_ADDED, GROUP_LABELS, RESET_ALL_LABEL, SOURCE_LABELS } from "../../constants/dashboard"
import type { DashboardBoardTemplate } from "../../types/dashboard"

// "Add board" while editing the dashboard: a blank board, every board a stack ships that is not on the dashboard now, and putting every board
// back as shipped. Its own component (and its own dismiss root) so it does not close the card catalogue beside it, or the other way round.
defineProps<{ offered: DashboardBoardTemplate[]; atLimit: boolean }>()
const emit = defineEmits<{
  (e: "add", template?: DashboardBoardTemplate): void
  (e: "reset-all"): void
}>()

const open = ref(false)
const root = ref<HTMLElement | null>(null)
useDismissable(root, open)
const offerLabel = (t: DashboardBoardTemplate) => (t.group ? GROUP_LABELS[t.group] : t.name)

function add(template?: DashboardBoardTemplate) {
  open.value = false
  emit("add", template)
}
</script>

<template>
  <div ref="root" class="relative">
    <BaseButton variant="secondary" size="sm" icon="plus" :disabled="atLimit" class-name="max-sm:!h-11" data-test="add-board" :aria-expanded="open" aria-controls="add-board-menu" @click="open = !open">{{ ADD_BOARD_LABEL }}</BaseButton>
    <PopoverPanel v-if="open" id="add-board-menu" :title="ADD_BOARD_LABEL" @close="open = false">
      <ul class="m-0 flex list-none flex-col gap-1 p-2">
        <li><BaseButton variant="ghost" class-name="w-full justify-start max-sm:!h-11" data-test="add-blank-board" @click="add()">Blank board</BaseButton></li>
        <li class="px-2 pt-2"><MicroLabel>Ready-made, from a stack</MicroLabel></li>
        <li v-for="t in offered" :key="t.id" class="flex items-center justify-between gap-2">
          <BaseButton variant="ghost" class-name="flex-1 justify-start max-sm:!h-11" :data-test="`add-board-${t.id}`" @click="add(t)">{{ offerLabel(t) }}</BaseButton>
          <span v-if="t.source" class="pr-2 text-[length:var(--text-small)] text-muted">{{ SOURCE_LABELS[t.source] }}</span>
        </li>
        <li v-if="!offered.length" class="px-2 py-1 text-[length:var(--text-small)] text-muted">{{ BOARD_ALREADY_ADDED }}: every stack board.</li>
        <li class="mt-1 border-t border-hairline px-0 pt-1"><BaseButton variant="ghost" class-name="w-full justify-start max-sm:!h-11" data-test="reset-all-boards" @click="open = false; emit('reset-all')">{{ RESET_ALL_LABEL }}</BaseButton></li>
      </ul>
    </PopoverPanel>
  </div>
</template>
