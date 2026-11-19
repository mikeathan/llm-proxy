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
import { CARD_DEFS, CARD_DEFS_BY_ID, KNOWN_CARD_IDS } from "../components/dashboard/cardRegistry"
import { useEnabledStacks } from "../composables/stacks/useEnabledStacks"
import { useDashboard } from "../composables/dashboard/useDashboard"
import { cardsToOffer, templateOf } from "../domain/dashboard"
import { CARD_MISSING, CARD_MISSING_ADD, DASHBOARD_EYEBROW, DASHBOARD_SUBTITLE, DASHBOARD_TITLE, MAX_BOARDS } from "../constants/dashboard"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import type { DashboardCardDef } from "../types/dashboard"

// The dashboard: boards (tabs) of cards. Home mixes cards from every part of the system; a stack ships its own board from the same cards.
// The stack pages keep control and settings and link here for the views. The layout belongs to the server, so it follows the owner to every
// browser; the board that is open is remembered per browser, and `?board=` opens one (the stack pages link to theirs).
const route = useRoute()
const stacks = useEnabledStacks()
const d = useDashboard(KNOWN_CARD_IDS, stacks.on)
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

function addCard(def: DashboardCardDef) {
  d.add({ id: def.id, cols: def.cols, rows: def.rows })
}

onMounted(async () => {
  await Promise.all([d.load(), stacks.load()])
  openFromAddress()
})
watch(() => route.query.board, openFromAddress)
function openFromAddress() {
  const id = route.query.board
  if (typeof id === "string") d.open(id)
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
      />
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
