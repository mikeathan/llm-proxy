<script setup lang="ts">
import { useId } from "vue"
import Icon from "../../icons/Icon.vue"

// A card whose header folds it. The title (a tag, a name, a few counts) is the toggle; `aside` (a "⋯" button, a status) sits
// beside it rather than inside it, so a control is never nested in the toggle button. Neutral surface only: state is said by
// the tags the caller puts in the title, never by the card. The body is not drawn while the card is folded.
const props = defineProps<{ open: boolean; label?: string }>()
const emit = defineEmits<{ (e: "update:open", open: boolean): void }>()
defineSlots<{ title(): unknown; aside?(): unknown; default(): unknown }>()

const bodyId = useId()
const toggle = () => emit("update:open", !props.open)
</script>

<template>
  <article :aria-label="label" class="min-w-0 rounded-[var(--radius-md)] border border-hairline bg-surface">
    <div :class="['flex min-h-11 items-center gap-2 pr-2', open ? 'border-b border-hairline' : '']">
      <button
        type="button"
        :aria-expanded="open"
        :aria-controls="open ? bodyId : undefined"
        data-test="card-toggle"
        class="flex min-h-11 min-w-0 flex-1 items-center gap-2.5 rounded-[var(--radius-md)] px-3 py-1.5 text-left hover:bg-surface-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-focus-ring"
        @click="toggle"
      >
        <Icon name="chevron-right" size="xs" :class-name="['flex-none text-muted transition-transform duration-[var(--motion-fast)]', open ? 'rotate-90' : ''].join(' ')" />
        <span class="flex min-w-0 flex-1 flex-wrap items-center gap-x-2.5 gap-y-1"><slot name="title" /></span>
      </button>
      <div v-if="$slots.aside" class="flex flex-none items-center gap-1"><slot name="aside" /></div>
    </div>
    <div v-if="open" :id="bodyId" class="min-w-0"><slot /></div>
  </article>
</template>
