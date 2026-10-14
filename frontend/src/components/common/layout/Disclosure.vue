<script setup lang="ts">
import { useId } from "vue"
import Icon from "../../icons/Icon.vue"

// A section that folds to one line: title and a summary of what is inside, the
// body shown on demand. `aside` (a status tag, a switch) sits beside the toggle
// rather than inside it, so interactive controls are never nested in the button.
// `expandable: false` is for a section with nothing to open: it keeps the line
// but has no toggle.
const props = withDefaults(defineProps<{ title: string; summary?: string; open: boolean; expandable?: boolean }>(), {
  summary: "",
  expandable: true,
})
const emit = defineEmits<{ (e: "update:open", open: boolean): void }>()
defineSlots<{ default(): unknown; aside?(): unknown }>()

const buttonId = useId()
const bodyId = useId()
const toggle = () => emit("update:open", !props.open)
</script>

<template>
  <section class="min-w-0 rounded-[var(--radius-md)] bg-canvas">
    <div class="flex min-h-11 items-center gap-3 pr-3">
      <button
        v-if="expandable"
        :id="buttonId"
        type="button"
        :aria-expanded="open"
        :aria-controls="bodyId"
        class="flex min-h-11 min-w-0 flex-1 items-center gap-3 rounded-[var(--radius-md)] px-3 text-left hover:bg-surface-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-focus-ring"
        @click="toggle"
      >
        <Icon name="chevron-right" size="xs" :class="['flex-none text-muted transition-transform duration-[var(--motion-fast)]', open ? 'rotate-90' : '']" />
        <span class="flex min-w-0 flex-1 flex-col gap-0.5 py-1.5 sm:flex-row sm:items-center sm:gap-3">
          <span class="flex-none font-mono text-[length:var(--text-small)] font-medium uppercase tracking-[0.04em] text-primary">{{ title }}</span>
          <span v-if="summary" class="min-w-0 truncate text-[length:var(--text-small)] text-muted">{{ summary }}</span>
        </span>
      </button>
      <div v-else class="flex min-h-11 min-w-0 flex-1 items-center gap-3 px-3">
        <span aria-hidden="true" class="w-3 flex-none"></span>
        <span class="flex min-w-0 flex-1 flex-col gap-0.5 py-1.5 sm:flex-row sm:items-center sm:gap-3">
          <span class="flex-none font-mono text-[length:var(--text-small)] font-medium uppercase tracking-[0.04em] text-primary">{{ title }}</span>
          <span v-if="summary" class="min-w-0 truncate text-[length:var(--text-small)] text-muted">{{ summary }}</span>
        </span>
      </div>
      <div v-if="$slots.aside" class="flex flex-none items-center gap-2"><slot name="aside" /></div>
    </div>
    <div v-if="expandable" v-show="open" :id="bodyId" role="region" :aria-labelledby="buttonId" class="flex flex-col gap-4 px-4 pb-4 pt-1">
      <slot />
    </div>
  </section>
</template>
