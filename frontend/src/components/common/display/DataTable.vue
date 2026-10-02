<script setup lang="ts" generic="T">
import type { DataTableColumn } from "../../../types/ui"
import EmptyState from "../feedback/EmptyState.vue"
import ErrorState from "../feedback/ErrorState.vue"
import LoadingState from "../feedback/LoadingState.vue"

// Column-defined table (Phase 5 primitive): tabular numerals, a sticky header,
// and below `sm` each row stacks into a labelled card (the header is visually
// hidden and every cell shows its column label). Loading, error and empty
// replace the rows. With `activatable`, rows are focusable and Enter / click
// emits `activate`; a `header-<key>` slot replaces a column's header text (the
// label stays the card label on mobile); a link or control inside a row keeps its own clicks and keys.

const props = withDefaults(
  defineProps<{
    columns: DataTableColumn<T>[]
    rows: T[]
    rowKey: (row: T) => string
    /** Names the table for assistive tech (visually hidden). */
    caption: string
    loading?: boolean
    /** The cause of a failed load; replaces the rows. */
    error?: string | null
    errorNext?: string
    emptyTitle?: string
    emptyBody?: string
    activatable?: boolean
  }>(),
  {
    loading: false,
    error: null,
    errorNext: "Try again in a moment; if it keeps failing, check the server log.",
    emptyTitle: "Nothing here yet",
    emptyBody: "",
    activatable: false,
  },
)

const emit = defineEmits<{ (e: "activate", row: T): void }>()

defineSlots<
  { [K in `cell-${string}`]?: (props: { row: T }) => unknown } & { [K in `header-${string}`]?: () => unknown } & {
    empty?(): unknown
  }
>()

const cellText = (column: DataTableColumn<T>, row: T) => {
  const value = column.value?.(row)
  return value === null || value === undefined ? "" : String(value)
}

// What a click inside a row may land on that is not the row itself.
const ROW_CONTROLS = "a, button, input, select, textarea, label"

function onClick(event: MouseEvent, row: T) {
  if (!props.activatable) return
  const control = event.target instanceof Element ? event.target.closest(ROW_CONTROLS) : null
  if (control && event.currentTarget instanceof Element && event.currentTarget.contains(control)) return
  emit("activate", row)
}

function onKeydown(event: KeyboardEvent, row: T) {
  // Keys pressed on a control inside the row (a copy button) are its own.
  if (event.target !== event.currentTarget) return
  if (props.activatable && event.key === "Enter") emit("activate", row)
}
</script>

<template>
  <LoadingState v-if="loading" :label="`Loading ${caption.toLowerCase()}`" />
  <ErrorState v-else-if="error" :title="`Could not load ${caption.toLowerCase()}`" :cause="error" :next="errorNext" />
  <slot v-else-if="!rows.length" name="empty">
    <EmptyState :title="emptyTitle" :body="emptyBody || undefined" />
  </slot>
  <div v-else class="max-w-full overflow-x-auto">
    <table class="w-full border-collapse max-sm:block">
      <caption class="sr-only">{{ caption }}</caption>
      <thead class="max-sm:sr-only">
        <tr>
          <th
            v-for="column in columns"
            :key="column.key"
            scope="col"
            :class="[
              'sticky top-0 z-[1] whitespace-nowrap border-b border-hairline bg-surface px-4 py-2 font-mono text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)] text-muted',
              column.numeric ? 'text-right' : 'text-left',
            ]"
          ><slot :name="`header-${column.key}`">{{ column.label }}</slot></th>
        </tr>
      </thead>
      <tbody class="max-sm:block">
        <tr
          v-for="row in rows"
          :key="rowKey(row)"
          :tabindex="activatable ? 0 : undefined"
          :class="[
            'group border-b border-hairline last:border-b-0 max-sm:block max-sm:px-3.5 max-sm:py-2.5',
            activatable ? 'cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset' : '',
          ]"
          @click="onClick($event, row)"
          @keydown="onKeydown($event, row)"
        >
          <td
            v-for="column in columns"
            :key="column.key"
            :data-label="column.label"
            :class="[
              'px-4 py-[9px] align-middle text-secondary group-hover:bg-surface-raised',
              'max-sm:flex max-sm:justify-between max-sm:gap-3 max-sm:px-0 max-sm:py-[3px] max-sm:text-right max-sm:group-hover:bg-transparent',
              'max-sm:before:flex-none max-sm:before:text-left max-sm:before:font-mono max-sm:before:text-[length:var(--text-micro)] max-sm:before:uppercase max-sm:before:leading-5 max-sm:before:tracking-[var(--tracking-micro)] max-sm:before:text-muted max-sm:before:content-[attr(data-label)]',
              column.numeric ? 'text-right font-mono text-[length:var(--text-small)] tabular-nums' : '',
            ]"
          >
            <slot :name="`cell-${column.key}`" :row="row">{{ cellText(column, row) }}</slot>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
