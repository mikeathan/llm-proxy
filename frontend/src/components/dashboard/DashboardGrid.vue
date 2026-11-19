<script setup lang="ts">
import { computed, ref } from "vue"
import DashboardCardFrame from "./DashboardCardFrame.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import { EMPTY_BOARD_BODY, EMPTY_BOARD_TITLE } from "../../constants/dashboard"
import type { DashboardBoard, DashboardCardDef, DashboardCols, DashboardRows } from "../../types/dashboard"

// The cards of one board on a grid of twelve columns. Below the tablet width every card is full width, one row tall, so a phone reads
// one card after another. While arranging, a card can be dragged to another place; the arrows on each card do the same from the
// keyboard. The grid only places cards: it never reads their data.
const props = defineProps<{ board: DashboardBoard; defs: ReadonlyMap<string, DashboardCardDef>; arrange: boolean; pinned: (cardId: string) => boolean }>()
const emit = defineEmits<{
  (e: "resize", cardId: string, size: { cols?: DashboardCols; rows?: DashboardRows }): void
  (e: "move", from: number, to: number): void
  (e: "remove", cardId: string): void
  (e: "pin", cardId: string): void
}>()

// A placement whose card the screen cannot draw is skipped (the layout pruning already drops it; this is the grid's own guard).
const shown = computed(() => props.board.cards.map((placement, index) => ({ placement, index, def: props.defs.get(placement.id) })).filter((c) => c.def))
const onHome = computed(() => props.board.id === "home")
const dragging = ref<number | null>(null)
const over = ref<number | null>(null)

function drop(to: number) {
  if (dragging.value !== null && dragging.value !== to) emit("move", dragging.value, to)
  dragging.value = null
  over.value = null
}
</script>

<template>
  <EmptyState v-if="!shown.length" :title="EMPTY_BOARD_TITLE" :body="EMPTY_BOARD_BODY" />
  <div v-else class="dash-grid" data-test="dashboard-grid">
    <div
      v-for="c in shown"
      :key="c.placement.id"
      :class="['dash-cell', { 'dash-cell--over': over === c.index && dragging !== null && dragging !== c.index, 'opacity-40': dragging === c.index }]"
      :style="{ '--cols': c.placement.cols, '--rows': c.placement.rows }"
      :draggable="arrange"
      @dragstart="dragging = c.index"
      @dragend="dragging = null; over = null"
      @dragover.prevent="over = c.index"
      @dragleave="over = null"
      @drop.prevent="drop(c.index)"
    >
      <DashboardCardFrame
        :def="c.def!"
        :placement="c.placement"
        :arrange="arrange"
        :index="c.index"
        :count="board.cards.length"
        :pinned="pinned(c.placement.id)"
        :on-home="onHome"
        @resize="(size) => emit('resize', c.placement.id, size)"
        @move="(delta) => emit('move', c.index, c.index + delta)"
        @remove="emit('remove', c.placement.id)"
        @pin="emit('pin', c.placement.id)"
      />
    </div>
  </div>
</template>

<style scoped>
.dash-grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  grid-auto-flow: row dense;
  grid-auto-rows: minmax(9rem, auto);
  gap: 1rem;
}
.dash-cell {
  grid-column: span 12;
  min-width: 0;
}
@media (min-width: 768px) {
  .dash-cell {
    grid-column: span var(--cols);
    grid-row: span var(--rows);
  }
}
.dash-cell--over {
  outline: 2px solid rgb(var(--accent-brand));
  outline-offset: 2px;
}
</style>
