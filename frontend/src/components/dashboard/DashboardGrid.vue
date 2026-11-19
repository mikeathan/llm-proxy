<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue"
import DashboardCardFrame from "./DashboardCardFrame.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import { EMPTY_BOARD_BODY, EMPTY_BOARD_TITLE } from "../../constants/dashboard"
import type { DashboardBoard, DashboardCardDef, DashboardCols, DashboardRows } from "../../types/dashboard"

// The cards of one board on a grid of twelve columns. Below the tablet width every card is full width, one row tall, so a phone reads
// one card after another. While arranging, a card can be dragged to another place; the arrows on each card do the same from the
// keyboard. The grid only places cards: it never reads their data. Cards are packed like bricks: each takes the highest place its width fits, in board
// order, so a short card leaves no hole under it and moving a card changes only where it and the cards after it sit. A card is as tall as its content (1×) or
// at least 28 rem (2×), so resizing one never stretches the cards beside it. How it packs: the grid's rows are 4 px, and each cell spans as many as its
// measured height needs (see pitfall 75).
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

// The rows are thin (ROW_UNIT_PX) and a cell spans as many as its own height needs, with the gap between cards counted in. A card that is
// taller or shorter changes only its own span, so the cards beside it never stretch; the cards after it settle into the room that is left.
const ROW_UNIT_PX = 4
const GAP_PX = 16
const spans = ref<Record<string, number>>({})
const cells = new Map<string, HTMLElement>()
const measure = (el: HTMLElement) => {
  const id = el.dataset.cardId
  if (id) spans.value[id] = Math.max(1, Math.ceil((el.getBoundingClientRect().height + GAP_PX) / ROW_UNIT_PX))
}
let observer: ResizeObserver | null = null
onMounted(() => {
  if (typeof ResizeObserver === "undefined") return
  observer = new ResizeObserver((entries) => entries.forEach((e) => e.target instanceof HTMLElement && measure(e.target)))
  cells.forEach((el) => observer?.observe(el))
})
onBeforeUnmount(() => observer?.disconnect())
function track(id: string, el: unknown) {
  const known = cells.get(id)
  if (known && known !== el) observer?.unobserve(known)
  if (!(el instanceof HTMLElement)) return void cells.delete(id)
  cells.set(id, el)
  observer?.observe(el)
  if (observer) measure(el)
}

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
      :ref="(el) => track(c.placement.id, el)"
      :data-card-id="c.placement.id"
      :class="['dash-cell', { 'dash-cell--tall': c.placement.rows > 1, 'dash-cell--over': over === c.index && dragging !== null && dragging !== c.index, 'opacity-40': dragging === c.index }]"
      :style="{ '--cols': c.placement.cols, '--span': spans[c.placement.id] ?? 1 }"
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
  grid-auto-rows: 4px;
  align-items: start;
  column-gap: 1rem;
  row-gap: 0;
}
.dash-cell {
  display: grid;
  grid-column: span 12;
  grid-row-end: span var(--span);
  min-width: 0;
}
@media (min-width: 768px) {
  .dash-cell {
    grid-column: span var(--cols);
  }
  /* Two rows tall is a minimum height: a short card gets room to spare, a long card (the radar) keeps all of its content with no scroll of its own.
     The cell's span is measured from its own height, so it cannot stretch the cards beside it. */
  .dash-cell--tall {
    min-height: 28rem;
  }
}
.dash-cell--over {
  outline: 2px solid rgb(var(--accent-brand));
  outline-offset: 2px;
}
</style>
