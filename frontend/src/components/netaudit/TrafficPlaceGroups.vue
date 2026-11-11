<script setup lang="ts">
import { formatBytes } from "../../utils/format/formatters"
import { whereShown } from "../../utils/netaudit/places"
import { GROUPS_CLOSE, GROUPS_NEW_CHIP, GROUPS_OPEN, GROUPS_PLACES, GROUPS_WHERE_MORE, ROUTER_CONN_COUNT } from "../../constants/netaudit"
import type { PlaceGroup, PlaceMeasure } from "../../types/netaudit"

// The company (or country) rows of a device: the name, where it is, how much it carried as a bar and a figure. A row picks the group. On a phone
// the group opens right under its row (the `body` slot); on a wide screen the pane shows the picked group beside the list, so the slot is unused.
const props = defineProps<{ groups: readonly PlaceGroup[]; measure: PlaceMeasure; selected: string | null; inline: boolean }>()
const emit = defineEmits<{ (e: "select", key: string): void }>()
defineSlots<{ body?(props: { group: PlaceGroup }): unknown }>()

const weight = (g: PlaceGroup): number => (props.measure === "bytes" ? g.bytes : g.count)
const biggest = (): number => Math.max(1, ...props.groups.map(weight))
const barWidth = (g: PlaceGroup): string => `${Math.max(3, Math.round((weight(g) / biggest()) * 100))}%`
const figure = (g: PlaceGroup): string => (props.measure === "bytes" ? formatBytes(g.bytes) : String(g.count))
const figureLabel = (g: PlaceGroup): string => (props.measure === "bytes" ? formatBytes(g.bytes) : ROUTER_CONN_COUNT(g.count))
const whereLine = (g: PlaceGroup): string => {
  const { shown, more } = whereShown(g.where)
  return [shown.join(", "), more ? GROUPS_WHERE_MORE(more) : ""].filter(Boolean).join(" ")
}
</script>

<template>
  <ul class="m-0 flex list-none flex-col p-0" data-test="place-groups">
    <li v-for="g in groups" :key="g.key || g.title" class="border-t border-hairline first:border-t-0" :data-selected="selected === g.key" data-test="place-group">
      <button
        type="button"
        class="flex w-full min-w-0 flex-col gap-1 border-l-2 bg-transparent px-3 py-2 text-left hover:bg-surface-hover max-sm:min-h-11"
        :class="selected === g.key ? 'border-accent-brand bg-surface-hover' : 'border-transparent'"
        :aria-expanded="selected === g.key"
        :aria-label="selected === g.key ? GROUPS_CLOSE(g.title) : GROUPS_OPEN(g.title)"
        data-test="group-open"
        @click="emit('select', g.key)"
      >
        <span class="flex w-full items-baseline justify-between gap-3">
          <span class="min-w-0 break-words text-primary" :class="selected === g.key ? 'font-medium' : ''"><span data-test="group-title">{{ g.title }}</span> <span v-if="g.fresh" class="ml-1 rounded-full border border-state-running px-1.5 text-[length:var(--text-micro)] text-state-running" data-test="group-new">{{ GROUPS_NEW_CHIP }}</span></span>
          <span class="flex-none font-mono text-[length:var(--text-small)] text-muted" :aria-label="figureLabel(g)" data-test="group-figure">{{ figure(g) }}</span>
        </span>
        <span class="flex w-full items-center gap-3">
          <span class="min-w-0 flex-1 break-words text-[length:var(--text-small)] text-muted" data-test="group-where">{{ whereLine(g) }} · {{ GROUPS_PLACES(g.rows.length) }}</span>
          <span class="block h-1.5 w-20 flex-none bg-surface-hover" aria-hidden="true"><span class="block h-1.5 bg-accent-info" :style="{ width: barWidth(g) }"></span></span>
        </span>
      </button>
      <div v-if="inline && selected === g.key" class="px-3 pb-3" data-test="group-body"><slot name="body" :group="g" /></div>
    </li>
  </ul>
</template>
