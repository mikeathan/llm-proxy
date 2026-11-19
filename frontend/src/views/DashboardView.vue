<script setup lang="ts">
import { computed, onMounted, watch } from "vue"
import { useRoute } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import Callout from "../components/common/feedback/Callout.vue"
import DashboardBoardTabs from "../components/dashboard/DashboardBoardTabs.vue"
import DashboardToolbar from "../components/dashboard/DashboardToolbar.vue"
import DashboardGrid from "../components/dashboard/DashboardGrid.vue"
import CardCatalogue from "../components/dashboard/CardCatalogue.vue"
import DashboardStackChrome from "../components/dashboard/DashboardStackChrome.vue"
import { CARD_DEFS, CARD_DEFS_BY_ID, KNOWN_CARD_IDS } from "../components/dashboard/cardRegistry"
import { useEnabledStacks } from "../composables/stacks/useEnabledStacks"
import { useDashboard } from "../composables/dashboard/useDashboard"
import { cardsToOffer, newShippedCards, templateOf } from "../domain/dashboard"
import { CARD_MISSING, CARD_MISSING_ADD, DASHBOARD_EYEBROW, DASHBOARD_SUBTITLE, DASHBOARD_TITLE, MAX_BOARDS, RESET_ALL_CONFIRM, NEW_CARDS_ADD, NEW_CARDS_DISMISS, NEW_CARDS_TITLE } from "../constants/dashboard"
import { useConfirm } from "../composables/ui/useConfirm"
import { usePersistedState } from "../composables/ui/usePersistedState"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import type { DashboardCardDef } from "../types/dashboard"

// The dashboard: boards (tabs) of cards. Home mixes cards from every part of the system; a stack ships its own board from the same cards.
// The stack pages keep control and settings and link here for the views. The layout belongs to the server, so it follows the owner to every
// browser; the board that is open is remembered per browser, and `?board=` opens one (the stack pages link to theirs).
const route = useRoute()
const stacks = useEnabledStacks()
const d = useDashboard(KNOWN_CARD_IDS, stacks.on, CARD_DEFS_BY_ID)
// A card or board of a stack with nothing switched on is not offered (one already on a board stays, and says it is off itself).
const offeredCards = computed(() => cardsToOffer(CARD_DEFS, stacks.on.value))
const onBoard = computed(() => new Set(d.board.value?.cards.map((c) => c.id)))
const canReset = computed(() => !!d.board.value && !!templateOf(d.board.value.id) && d.preset.value !== "default")

// A stack page can link to one card (`?card=`). The layout is the owner's, so the card may not be on their saved board (it was added after the board was
// saved): say so, with the way to add it, instead of leaving a link that shows nothing.
const wantedCard = computed(() => {
  const id = route.query.card
  return typeof id === "string" ? (CARD_DEFS_BY_ID.get(id) ?? null) : null
})
const missingCard = computed(() => (wantedCard.value && d.board.value && !onBoard.value.has(wantedCard.value.id) ? wantedCard.value : null))

// A saved copy of a shipped board does not have the cards added since it was saved: say so once, and let the owner add them or not. What was offered
// is remembered per browser, so a card they decline (or removed on purpose) does not come back.
const SEEN_VERSION = 1
const offered = usePersistedState<Record<string, string[]>>("dashboard-offered-cards", {
  version: SEEN_VERSION,
  parse: (data) => (data && typeof data === "object" && !Array.isArray(data) ? (data as Record<string, string[]>) : null),
  fallback: {},
})
const newCards = computed(() => (d.board.value ? newShippedCards(d.board.value, offered.value[d.board.value.id] ?? [], stacks.on.value, CARD_DEFS_BY_ID) : []))
function settleNewCards(add: boolean) {
  const board = d.board.value
  if (!board) return
  for (const id of newCards.value) {
    const def = CARD_DEFS_BY_ID.get(id)
    if (add && def) addCard(def)
  }
  offered.value = { ...offered.value, [board.id]: [...(offered.value[board.id] ?? []), ...newCards.value] }
}

const { confirm } = useConfirm()
async function resetEverything() {
  if (await confirm({ ...RESET_ALL_CONFIRM, type: "warning" })) await d.resetAll()
}

function addCard(def: DashboardCardDef) {
  d.add({ id: def.id, cols: def.cols, rows: def.rows })
}

onMounted(async () => {
  await Promise.all([d.load(), stacks.load()])
  openFromAddress()
})
watch(() => route.query.board, openFromAddress)
// A stack page links to its board. A layout saved before that board shipped does not have it, so following the link adds the shipped board
// (the owner asked for it by following the link; the same board is in the Add board menu). A board that is not shipped is not made up.
function openFromAddress() {
  const id = route.query.board
  if (typeof id !== "string") return
  const shipped = d.boards.value.some((b) => b.id === id) ? undefined : d.offeredBoards.value.find((t) => t.id === id)
  if (shipped) d.addBoardFrom(shipped)
  else d.open(id)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :eyebrow="DASHBOARD_EYEBROW" :title="DASHBOARD_TITLE" :subtitle="DASHBOARD_SUBTITLE" subtitle-hidden-on-phone />
    <DashboardToolbar v-if="d.board.value" :preset="d.preset.value" :arrange="d.arrange.value" :can-reset="canReset" @preset="d.setPreset" @arrange="d.arrange.value = $event" @reset="d.resetBoard">
      <template #catalogue="{ close }">
        <CardCatalogue :defs="offeredCards" :on-board="onBoard" @add="(def) => { addCard(def); close() }" @close="close" />
      </template>
    </DashboardToolbar>

    <Callout v-if="missingCard" tone="info" :title="CARD_MISSING(missingCard.title)">
      <template #actions><BaseButton variant="secondary" class-name="max-sm:!h-11" data-test="add-wanted-card" @click="addCard(missingCard)">{{ CARD_MISSING_ADD }}</BaseButton></template>
    </Callout>
    <Callout v-if="newCards.length" tone="info" :title="NEW_CARDS_TITLE(newCards.map((id) => CARD_DEFS_BY_ID.get(id)!.title))">
      <template #actions>
        <BaseButton variant="secondary" class-name="max-sm:!h-11" data-test="add-new-cards" @click="settleNewCards(true)">{{ NEW_CARDS_ADD }}</BaseButton>
        <BaseButton variant="ghost" class-name="max-sm:!h-11" data-test="dismiss-new-cards" @click="settleNewCards(false)">{{ NEW_CARDS_DISMISS }}</BaseButton>
      </template>
    </Callout>
    <Callout v-if="d.error.value" tone="warning" title="Showing the standard boards"><span data-test="dashboard-error">{{ d.error.value }}</span></Callout>
    <LoadingState v-if="d.loading.value && !d.layout.value" label="Loading the dashboard" :rows="3" />
    <template v-else-if="d.board.value">
      <DashboardBoardTabs
        :boards="d.boards.value"
        :active-id="d.board.value.id"
        :arrange="d.arrange.value"
        :offered="d.offeredBoards.value"
        :at-limit="d.boards.value.length >= MAX_BOARDS"
        @open="d.open"
        @add="d.addBoardFrom"
        @remove="d.removeBoardById"
        @rename="d.rename"
        @reset-all="resetEverything"
      />
      <DashboardStackChrome :board="d.board.value" :defs="CARD_DEFS_BY_ID" />
      <DashboardGrid
        :board="d.board.value"
        :defs="CARD_DEFS_BY_ID"
        :arrange="d.arrange.value"
        :pinned="d.pinned"
        @resize="d.resize"
        @move="d.move"
        @remove="d.remove"
        @pin="d.pin"
      />
    </template>
  </div>
</template>
