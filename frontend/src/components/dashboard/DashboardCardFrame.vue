<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, provide, ref } from "vue"
import DashboardCardBody from "./DashboardCardBody.vue"
import { useResponsiveLayout } from "../../composables/ui/useResponsiveLayout"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import { DASHBOARD_CARD_SIZE, DASHBOARD_CARD_VISIBLE, PINNED_LABEL, PIN_LABEL, REMOVE_CARD_LABEL, ROWS_OPTIONS, SIZE_OPTIONS, SOURCE_CHIPS } from "../../constants/dashboard"
import { sizeForWidth } from "../../domain/dashboard"
import type { DashboardCardDef, DashboardCardSize, DashboardCols, DashboardPlacement, DashboardRows } from "../../types/dashboard"

// One card of the dashboard: the frame every card shares. It draws the title, the source chip and the arrow to the control page, and
// (while the board is being arranged) the controls for width, height, order, pinning and removing. What the card shows is its own
// component, which fills the room it is given. The frame measures the card's own width and provides its size (small, medium or large), so
// a card is built once and shows the version that fits; only that version is drawn.
const props = defineProps<{ def: DashboardCardDef; placement: DashboardPlacement; arrange: boolean; index: number; count: number; pinned: boolean; onHome: boolean }>()
const emit = defineEmits<{
  (e: "resize", size: { cols?: DashboardCols; rows?: DashboardRows }): void
  (e: "move", delta: number): void
  (e: "remove"): void
  (e: "pin"): void
}>()

const sizeOptions = SIZE_OPTIONS.map((s) => ({ value: String(s.cols), label: s.label }))
const rowOptions = ROWS_OPTIONS.map((r) => ({ value: String(r.rows), label: r.label }))
const control = computed(() => props.def.control())

// A move re-orders the cards, and the browser drops focus from a node that is moved. Put it back on the button that was used (or on the
// other one when this one has reached the end) and keep the card in view, so pressing "later" again and again works.
const controls = ref<HTMLElement | null>(null)
async function move(delta: number) {
  emit("move", delta)
  await nextTick()
  const pick = (name: string) => controls.value?.querySelector<HTMLButtonElement>(`[data-move="${name}"]`) ?? null
  const used = pick(delta < 0 ? "earlier" : "later")
  const target = used && !used.disabled ? used : pick(delta < 0 ? "later" : "earlier")
  target?.focus({ preventScroll: true })
  controls.value?.scrollIntoView({ block: "nearest", behavior: "smooth" })
}

const body = ref<HTMLElement | null>(null)
// Below the tablet width the grid is one column and every card is its own full-width block with all the height it needs, so it shows its
// full detail; from the tablet width up the card's own measured width decides.
const { breakpoint } = useResponsiveLayout()
const stacked = computed(() => breakpoint.value === "base" || breakpoint.value === "sm")
const measured = ref<DashboardCardSize>("lg")
const size = computed<DashboardCardSize>(() => (stacked.value ? "lg" : measured.value))
provide(DASHBOARD_CARD_SIZE, size)
// Whether the card is on screen: the one place that watches, so a poll inside a card needs no observer of its own. Before it is known (or
// where there is no IntersectionObserver) the card counts as visible.
const visible = ref(true)
provide(DASHBOARD_CARD_VISIBLE, visible)
let observer: ResizeObserver | undefined
let seen: IntersectionObserver | undefined
onMounted(() => {
  if (!body.value) return
  if (typeof ResizeObserver !== "undefined") {
    observer = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect.width
      if (width !== undefined) measured.value = sizeForWidth(width)
    })
    observer.observe(body.value)
  }
  if (typeof IntersectionObserver !== "undefined") {
    seen = new IntersectionObserver((entries) => {
      const last = entries[entries.length - 1]
      if (last) visible.value = last.isIntersecting
    })
    seen.observe(body.value)
  }
})
onBeforeUnmount(() => {
  observer?.disconnect()
  seen?.disconnect()
})
</script>

<template>
  <Panel :title="def.title" class="h-full" :data-test="`card-${def.id}`">
    <template #actions>
      <span class="rounded-[var(--radius-sm)] bg-surface-active px-1.5 py-0.5 font-mono text-[length:var(--text-micro)] tracking-[var(--tracking-micro)] text-secondary" data-test="card-source">{{ SOURCE_CHIPS[def.source] }}</span>
      <RouterLink :to="control" :aria-label="`Open ${def.title}: control and settings`" class="inline-flex min-h-8 items-center rounded-[var(--radius-sm)] px-1.5 font-mono text-muted hover:text-primary focus-visible:outline-none focus-visible:ring-2 max-sm:min-h-11" data-test="card-open">↗</RouterLink>
    </template>
    <div v-if="arrange" ref="controls" class="mb-3 flex flex-wrap items-center gap-2 border-b border-hairline pb-3" data-test="card-controls">
      <SegmentedControl class="max-md:hidden" :model-value="String(placement.cols)" :options="sizeOptions" :label="`Width of ${def.title}`" variant="compact" @update:model-value="emit('resize', { cols: Number($event) as DashboardCols })" />
      <SegmentedControl class="max-md:hidden" :model-value="String(placement.rows)" :options="rowOptions" :label="`Height of ${def.title}`" variant="compact" @update:model-value="emit('resize', { rows: Number($event) as DashboardRows })" />
      <span class="flex items-center gap-1">
        <span data-drag-grip draggable="true" title="Drag to move" aria-hidden="true" class="inline-flex min-h-8 cursor-grab select-none items-center px-1.5 font-mono text-muted max-md:hidden">⠿</span>
        <BaseButton data-move="earlier" variant="ghost" size="sm" icon="chevron-left" icon-only :label="`Move ${def.title} earlier`" :disabled="index === 0" @click="move(-1)" />
        <BaseButton data-move="later" variant="ghost" size="sm" icon="chevron-right" icon-only :label="`Move ${def.title} later`" :disabled="index >= count - 1" @click="move(1)" />
      </span>
      <BaseButton v-if="!onHome" variant="ghost" size="sm" :disabled="pinned" data-test="card-pin" @click="emit('pin')">{{ pinned ? PINNED_LABEL : PIN_LABEL }}</BaseButton>
      <BaseButton variant="ghost" size="sm" icon="close" icon-only :label="`${REMOVE_CARD_LABEL}: ${def.title}`" data-test="card-remove" @click="emit('remove')" />
    </div>
    <div ref="body" class="min-w-0" :data-size="size">
      <DashboardCardBody><component :is="def.component" /></DashboardCardBody>
    </div>
  </Panel>
</template>

