<script setup lang="ts">
import { computed } from "vue"
import type { StatusState } from "../../../types/ui"
import { HOLLOW_DOT_STATES, STATUS_TAG_CLASS } from "../../../constants/status"

// Borderless tinted state tag with a square dot (hollow while queued). The
// label is always present — state is never conveyed by colour alone.
const props = defineProps<{ state: StatusState; label: string }>()
const hollow = computed(() => HOLLOW_DOT_STATES.includes(props.state))
</script>

<template>
  <span
    :class="[
      'inline-flex h-5 items-center gap-1.5 whitespace-nowrap px-[7px] font-mono text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)]',
      STATUS_TAG_CLASS[state],
    ]"
  >
    <span
      data-test="dot"
      aria-hidden="true"
      :class="['h-1.5 w-1.5 flex-none', hollow ? 'bg-transparent shadow-[inset_0_0_0_1px_currentColor]' : 'bg-current']"
    ></span>
    {{ label }}
  </span>
</template>
