<script setup lang="ts">
import { computed, ref } from "vue"
import ScrollStrip from "../common/layout/ScrollStrip.vue"
import PopoverPanel from "../common/layout/PopoverPanel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import { useDismissable } from "../../composables/ui/useDismissable"
import { useBriefUnread } from "../../composables/newsroom/useBriefUnread"
import { unreadBriefWords } from "../../constants/newsroom"
import { ADD_BOARD_LABEL, BOARD_ALREADY_ADDED, GROUP_LABELS, HOME_BOARD_ID, MAX_BOARD_NAME, REMOVE_BOARD_LABEL, RENAME_BOARD_LABEL, RESET_ALL_LABEL, SOURCE_LABELS, SUB_TABS_LABEL } from "../../constants/dashboard"
import { boardTabs, groupOf } from "../../domain/dashboard"
import type { DashboardBoard, DashboardBoardTemplate } from "../../types/dashboard"

// The boards of the dashboard, as tabs. Home comes first and stays. A stack that ships several boards (Network) is one tab with a row of sub-tabs under
// it, so the strip stays short however much a stack has to show. While arranging, the open board can be renamed or removed here, and "Add board"
// offers a blank board and every board a stack ships that is not on the dashboard now.
const props = defineProps<{ boards: DashboardBoard[]; activeId: string; arrange: boolean; offered: DashboardBoardTemplate[]; atLimit: boolean; stacksOn: ReadonlySet<string> | null }>()
defineSlots<{ trailing?(): unknown }>()
const emit = defineEmits<{
  (e: "open", id: string): void
  (e: "add", template?: DashboardBoardTemplate): void
  (e: "remove", id: string): void
  (e: "rename", id: string, name: string): void
  (e: "reset-all"): void
}>()

// A dot on the Newsroom tab while a written brief waits unread; the card inside it says which. The count is shared with the Newsroom page's tab.
const NEWSROOM_BOARD_ID = "newsroom"
const { unreadCount } = useBriefUnread()
const menuOpen = ref(false)
const root = ref<HTMLElement | null>(null)
useDismissable(root, menuOpen)
const active = () => props.boards.find((b) => b.id === props.activeId)
const tabs = computed(() => boardTabs(props.boards, props.stacksOn))
const activeGroup = computed(() => groupOf(props.activeId))
const subTabs = computed(() => tabs.value.find((t) => t.group && t.group === activeGroup.value)?.subTabs ?? [])
const isTabActive = (t: { id: string; group?: string }) => (t.group ? t.group === activeGroup.value : t.id === props.activeId)
const offerLabel = (t: DashboardBoardTemplate) => (t.group ? GROUP_LABELS[t.group] : t.name)
// A board a stack ships is named by the stack and is one view of its tab, so it is neither renamed nor removed here: the stack's tab goes with the stack
// (switched off), and "Put every board back as shipped" restores the views.
const renameable = computed(() => !activeGroup.value)

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
        <button v-for="t in tabs" :key="t.id" type="button" :aria-current="isTabActive(t) ? 'page' : undefined" class="dash-tab" data-test="board-tab" @click="emit('open', t.id)">{{ t.label }}<template v-if="t.id === NEWSROOM_BOARD_ID && unreadCount > 0"><span class="dash-tab-dot" aria-hidden="true" data-test="board-tab-dot" /><span class="sr-only">, {{ unreadBriefWords(unreadCount) }}</span></template></button>
      </ScrollStrip>
      <BaseButton variant="ghost" size="sm" icon="plus" :disabled="atLimit" class-name="flex-none max-sm:!h-11" data-test="add-board" :aria-expanded="menuOpen" aria-controls="add-board-menu" @click="menuOpen = !menuOpen">{{ ADD_BOARD_LABEL }}</BaseButton>
      <PopoverPanel v-if="menuOpen" id="add-board-menu" :title="ADD_BOARD_LABEL" @close="menuOpen = false">
        <ul class="m-0 flex list-none flex-col gap-1 p-2">
          <li><BaseButton variant="ghost" class-name="w-full justify-start max-sm:!h-11" data-test="add-blank-board" @click="add()">Blank board</BaseButton></li>
          <li class="px-2 pt-2"><MicroLabel>Ready-made, from a stack</MicroLabel></li>
          <li v-for="t in offered" :key="t.id" class="flex items-center justify-between gap-2">
            <BaseButton variant="ghost" class-name="flex-1 justify-start max-sm:!h-11" :data-test="`add-board-${t.id}`" @click="add(t)">{{ offerLabel(t) }}</BaseButton>
            <span v-if="t.source" class="pr-2 text-[length:var(--text-small)] text-muted">{{ SOURCE_LABELS[t.source] }}</span>
          </li>
          <li v-if="!offered.length" class="px-2 py-1 text-[length:var(--text-small)] text-muted">{{ BOARD_ALREADY_ADDED }}: every stack board.</li>
          <li class="mt-1 border-t border-hairline px-0 pt-1"><BaseButton variant="ghost" class-name="w-full justify-start max-sm:!h-11" data-test="reset-all-boards" @click="menuOpen = false; emit('reset-all')">{{ RESET_ALL_LABEL }}</BaseButton></li>
        </ul>
      </PopoverPanel>
    </div>
    <div v-if="subTabs.length" class="flex items-center justify-between gap-3 max-sm:flex-col max-sm:items-stretch" data-test="sub-tabs">
      <ScrollStrip :label="SUB_TABS_LABEL(GROUP_LABELS[activeGroup!])" :current="activeId" class="min-w-0 flex-1">
        <button v-for="s in subTabs" :key="s.id" type="button" :aria-current="s.id === activeId ? 'page' : undefined" class="dash-pill" data-test="sub-tab" @click="emit('open', s.id)">{{ s.label }}</button>
      </ScrollStrip>
      <div class="flex flex-none justify-end"><slot name="trailing" /></div>
    </div>
    <div v-if="arrange && activeId !== HOME_BOARD_ID && active() && renameable" class="flex flex-wrap items-center gap-2" data-test="board-edit">
      <label v-if="renameable" class="flex items-center gap-2 text-[length:var(--text-small)] text-secondary">{{ RENAME_BOARD_LABEL }}
        <input :value="active()!.name" :maxlength="MAX_BOARD_NAME" class="h-8 w-48 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2" data-test="board-name" @change="rename" />
      </label>
      <BaseButton v-if="renameable" variant="danger" size="sm" icon="trash" data-test="remove-board" @click="emit('remove', activeId)">{{ REMOVE_BOARD_LABEL }}</BaseButton>
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
.dash-tab-dot {
  @apply ml-2 inline-block h-2 w-2 flex-none rounded-full bg-accent-brand;
}
.dash-pill {
  @apply inline-flex h-11 flex-none items-center rounded-full border border-hairline bg-transparent px-3.5 text-[length:var(--text-small)] text-muted hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8;
}
.dash-pill[aria-current="page"] {
  @apply border-accent-brand bg-surface-hover font-medium text-primary;
}
</style>
