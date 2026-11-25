<script setup lang="ts">
import { computed } from "vue"
import ScrollStrip from "../common/layout/ScrollStrip.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useBriefUnread } from "../../composables/newsroom/useBriefUnread"
import { useAlertsUnread } from "../../composables/newsroom/useAlertsUnread"
import { unreadAlertsWords, unreadBriefWords } from "../../constants/newsroom"
import { GROUP_LABELS, HOME_BOARD_ID, NEWSROOM_BRIEF_BOARD, NEWSROOM_FEED_BOARD, MAX_BOARD_NAME, REMOVE_BOARD_LABEL, RENAME_BOARD_LABEL, SUB_TABS_LABEL } from "../../constants/dashboard"
import { boardTabs, groupOf } from "../../domain/dashboard"
import type { DashboardBoard } from "../../types/dashboard"

// The boards of the dashboard, as tabs. Home comes first and stays. A stack that ships several boards (Network) is one tab with a row of sub-tabs under
// it, so the strip stays short however much a stack has to show. While editing, the open board can be renamed or removed here; adding a board
// is in the edit bar (DashboardToolbar), so the tabs get the whole width.
const props = defineProps<{ boards: DashboardBoard[]; activeId: string; arrange: boolean; stacksOn: ReadonlySet<string> | null }>()
defineSlots<{ trailing?(): unknown }>()
const emit = defineEmits<{
  (e: "open", id: string): void
  (e: "remove", id: string): void
  (e: "rename", id: string, name: string): void
}>()

// A dot on the Brief sub-tab while a written brief waits unread, and on Feed while alerts do; the Newsroom tab has one while either does. The counts are
// the server's, shared with every other dot of the Newsroom.
const { unreadCount } = useBriefUnread()
const { unread: unreadAlerts } = useAlertsUnread()
const dots = computed<Record<string, string>>(() => ({
  ...(unreadCount.value > 0 ? { [NEWSROOM_BRIEF_BOARD]: unreadBriefWords(unreadCount.value) } : {}),
  ...(unreadAlerts.value > 0 ? { [NEWSROOM_FEED_BOARD]: unreadAlertsWords(unreadAlerts.value) } : {}),
}))
// A tab's dot: its own board's, or for a group every sub-tab's, said together.
const tabDot = (t: { id: string; subTabs: { id: string }[] }): string => (t.subTabs.length ? t.subTabs.map((s) => dots.value[s.id]).filter(Boolean).join(", ") : (dots.value[t.id] ?? ""))
const active = () => props.boards.find((b) => b.id === props.activeId)
const tabs = computed(() => boardTabs(props.boards, props.stacksOn))
const activeGroup = computed(() => groupOf(props.activeId))
const subTabs = computed(() => tabs.value.find((t) => t.group && t.group === activeGroup.value)?.subTabs ?? [])
const isTabActive = (t: { id: string; group?: string }) => (t.group ? t.group === activeGroup.value : t.id === props.activeId)
// A board a stack ships is named by the stack and is one view of its tab, so it is neither renamed nor removed here: the stack's tab goes with the stack
// (switched off), and "Put every board back as shipped" restores the views.
const renameable = computed(() => !activeGroup.value)

function rename(event: Event) {
  const board = active()
  if (board) emit("rename", board.id, (event.target as HTMLInputElement).value)
}
</script>

<template>
  <div class="flex flex-col gap-2">
    <!-- relative: the screen-reader words of the tab dot are absolutely placed and must stay inside the strip, not widen the page. -->
    <div class="relative flex items-center gap-2 border-b border-strong">
      <ScrollStrip label="Dashboard boards" :current="activeId" class="min-w-0 flex-1">
        <button v-for="t in tabs" :key="t.id" type="button" :aria-current="isTabActive(t) ? 'page' : undefined" class="dash-tab" data-test="board-tab" @click="emit('open', t.id)">{{ t.label }}<template v-if="tabDot(t)"><span class="dash-tab-dot" aria-hidden="true" data-test="board-tab-dot" /><span class="sr-only">, {{ tabDot(t) }}</span></template></button>
      </ScrollStrip>
    </div>
    <div v-if="subTabs.length" class="flex items-center justify-between gap-3 max-sm:flex-col max-sm:items-stretch" data-test="sub-tabs">
      <ScrollStrip :label="SUB_TABS_LABEL(GROUP_LABELS[activeGroup!])" :current="activeId" class="min-w-0 flex-1">
        <button v-for="s in subTabs" :key="s.id" type="button" :aria-current="s.id === activeId ? 'page' : undefined" class="dash-pill" data-test="sub-tab" @click="emit('open', s.id)">{{ s.label }}<template v-if="dots[s.id]"><span class="dash-tab-dot" aria-hidden="true" data-test="sub-tab-dot" /><span class="sr-only">, {{ dots[s.id] }}</span></template></button>
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
  @apply inline-flex h-11 flex-none items-center border-b-2 border-transparent bg-transparent px-3.5 text-muted max-sm:px-3 hover:text-primary focus-visible:outline-none focus-visible:ring-2;
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
