<script setup lang="ts">
import { computed, ref } from "vue"
import DashboardCardFrame from "./DashboardCardFrame.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import { EMPTY_BOARD_BODY, EMPTY_BOARD_TITLE } from "../../constants/dashboard"
import type { DashboardBoard, DashboardCardDef, DashboardCols, DashboardRows } from "../../types/dashboard"

// The cards of one board on a grid of twelve columns. Below the tablet width every card is full width, one row tall, so a phone reads
// one card after another. While arranging, a card can be dragged to another place; the arrows on each card do the same from the
// keyboard. The grid only places cards: it never reads their data. Cards flow in board order (no gap filling), so "earlier" and "later"
// always change what is seen; a card is as tall as its content (1×) or at least 28 rem (2×), so resizing one never stretches the cards beside it.
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
// Only the grip on a card starts a drag, so the card's own buttons, selects and text keep working.
function dragStart(index: number, e: DragEvent) {
  if (!(e.target instanceof HTMLElement) || !e.target.closest('[data-drag-grip]')) return
  dragging.value = index
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(index))
    if (e.currentTarget instanceof HTMLElement) e.dataTransfer.setDragImage(e.currentTarget, 16, 16)
  }
}
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
      :class="['dash-cell', { 'dash-cell--tall': c.placement.rows > 1, 'dash-cell--over': over === c.index && dragging !== null && dragging !== c.index, 'opacity-40': dragging === c.index }]"
      :style="{ '--cols': c.placement.cols }"
      @dragstart="dragStart(c.index, $event)"
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
  align-items: start;
  gap: 1rem;
}
.dash-cell {
  display: grid;
  grid-column: span 12;
  min-width: 0;
}
@media (min-width: 768px) {
  .dash-cell {
    grid-column: span var(--cols);
  }
  /* Two rows tall is a minimum height: a short card gets room to spare, a long card (the radar) keeps all of its content with no scroll of its own.
     It is never a span across rows, so it cannot stretch the cards beside it. */
  .dash-cell--tall {
    min-height: 28rem;
  }
}
.dash-cell--over {
  outline: 2px solid rgb(var(--accent-brand));
  outline-offset: 2px;
}
</style>
