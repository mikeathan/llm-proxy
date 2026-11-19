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
import { useDashboard } from "../composables/dashboard/useDashboard"
import { templateOf } from "../domain/dashboard"
import { DASHBOARD_EYEBROW, DASHBOARD_SUBTITLE, DASHBOARD_TITLE, MAX_BOARDS } from "../constants/dashboard"
import type { DashboardCardDef } from "../types/dashboard"

// The dashboard: boards (tabs) of cards. Home mixes cards from every part of the system; a stack ships its own board from the same cards.
// The stack pages keep control and settings and link here for the views. The layout belongs to the server, so it follows the owner to every
// browser; the board that is open is remembered per browser, and `?board=` opens one (the stack pages link to theirs).
const route = useRoute()
const d = useDashboard(KNOWN_CARD_IDS)
const onBoard = computed(() => new Set(d.board.value?.cards.map((c) => c.id)))
const canReset = computed(() => !!d.board.value && !!templateOf(d.board.value.id) && d.preset.value !== "default")

function addCard(def: DashboardCardDef) {
  d.add({ id: def.id, cols: def.cols, rows: def.rows })
}

onMounted(async () => {
  await d.load()
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
        <CardCatalogue :defs="CARD_DEFS" :on-board="onBoard" @add="(def) => { addCard(def); close() }" @close="close" />
      </template>
    </DashboardToolbar>

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
