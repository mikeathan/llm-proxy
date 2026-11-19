<script setup lang="ts">
import { ref } from "vue"
import ScrollStrip from "../common/layout/ScrollStrip.vue"
import PopoverPanel from "../common/layout/PopoverPanel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import { useDismissable } from "../../composables/ui/useDismissable"
import { ADD_BOARD_LABEL, BOARD_ALREADY_ADDED, HOME_BOARD_ID, MAX_BOARD_NAME, REMOVE_BOARD_LABEL, RENAME_BOARD_LABEL, RESET_ALL_LABEL, SOURCE_LABELS } from "../../constants/dashboard"
import type { DashboardBoard, DashboardBoardTemplate } from "../../types/dashboard"

// The boards of the dashboard, as tabs. Home comes first and stays. While arranging, the open board can be renamed or removed here, and
// "Add board" offers a blank board and every board a stack ships that is not on the dashboard now.
const props = defineProps<{ boards: DashboardBoard[]; activeId: string; arrange: boolean; offered: DashboardBoardTemplate[]; atLimit: boolean }>()
const emit = defineEmits<{
  (e: "open", id: string): void
  (e: "add", template?: DashboardBoardTemplate): void
  (e: "remove", id: string): void
  (e: "rename", id: string, name: string): void
  (e: "reset-all"): void
}>()

const menuOpen = ref(false)
const root = ref<HTMLElement | null>(null)
useDismissable(root, menuOpen)
const active = () => props.boards.find((b) => b.id === props.activeId)

function add(template?: DashboardBoardTemplate) {
  menuOpen.value = false
  emit("add", template)
}
function rename(event: Event) {
  const board = active()
  if (board) emit("rename", board.id, (event.target as HTMLInputElement).value)
}
</script>

<template>
  <div class="flex flex-col gap-2">
    <div ref="root" class="relative flex items-center gap-2 border-b border-strong">
      <ScrollStrip label="Dashboard boards" :current="activeId" class="min-w-0 flex-1">
        <button v-for="b in boards" :key="b.id" type="button" :aria-current="b.id === activeId ? 'page' : undefined" class="dash-tab" data-test="board-tab" @click="emit('open', b.id)">{{ b.name }}</button>
      </ScrollStrip>
      <BaseButton variant="ghost" size="sm" icon="plus" :disabled="atLimit" class-name="flex-none max-sm:!h-11" data-test="add-board" :aria-expanded="menuOpen" aria-controls="add-board-menu" @click="menuOpen = !menuOpen">{{ ADD_BOARD_LABEL }}</BaseButton>
      <PopoverPanel v-if="menuOpen" id="add-board-menu" :title="ADD_BOARD_LABEL" @close="menuOpen = false">
        <ul class="m-0 flex list-none flex-col gap-1 p-2">
          <li><BaseButton variant="ghost" class-name="w-full justify-start max-sm:!h-11" data-test="add-blank-board" @click="add()">Blank board</BaseButton></li>
          <li class="px-2 pt-2"><MicroLabel>Ready-made, from a stack</MicroLabel></li>
          <li v-for="t in offered" :key="t.id" class="flex items-center justify-between gap-2">
            <BaseButton variant="ghost" class-name="flex-1 justify-start max-sm:!h-11" :data-test="`add-board-${t.id}`" @click="add(t)">{{ t.name }}</BaseButton>
            <span v-if="t.source" class="pr-2 text-[length:var(--text-small)] text-muted">{{ SOURCE_LABELS[t.source] }}</span>
          </li>
          <li v-if="!offered.length" class="px-2 py-1 text-[length:var(--text-small)] text-muted">{{ BOARD_ALREADY_ADDED }}: every stack board.</li>
          <li class="mt-1 border-t border-hairline px-0 pt-1"><BaseButton variant="ghost" class-name="w-full justify-start max-sm:!h-11" data-test="reset-all-boards" @click="menuOpen = false; emit('reset-all')">{{ RESET_ALL_LABEL }}</BaseButton></li>
        </ul>
      </PopoverPanel>
    </div>
    <div v-if="arrange && activeId !== HOME_BOARD_ID && active()" class="flex flex-wrap items-center gap-2" data-test="board-edit">
      <label class="flex items-center gap-2 text-[length:var(--text-small)] text-secondary">{{ RENAME_BOARD_LABEL }}
        <input :value="active()!.name" :maxlength="MAX_BOARD_NAME" class="h-8 w-48 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2" data-test="board-name" @change="rename" />
      </label>
      <BaseButton variant="danger" size="sm" icon="trash" data-test="remove-board" @click="emit('remove', activeId)">{{ REMOVE_BOARD_LABEL }}</BaseButton>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.dash-tab {
  @apply inline-flex h-11 flex-none items-center border-b-2 border-transparent bg-transparent px-3.5 text-muted hover:text-primary focus-visible:outline-none focus-visible:ring-2;
}
.dash-tab[aria-current="page"] {
  @apply border-accent-brand font-medium text-primary;
}
</style>
