<script setup lang="ts">
import { computed, ref } from "vue"
import { usePolling } from "../../composables/ui/usePolling"
import { toRadio } from "../../router/routes"
import {
  RADIO_STATUS_IDLE, RADIO_STATUS_READING, RADIO_STATUS_START, radioStatusListening, radioStatusSorting,
} from "../../constants/radio"
import type { RadioLive } from "../../types/radio"

// Radio · Around you: one line that says whether this is live. Listening shows the time to the next reading (a dot moves once a cycle) and how far the
// sorting has got; not listening says the picture shows where devices usually are, with the way to start.
const props = defineProps<{ view: RadioLive }>()
const now = ref(Math.floor(Date.now() / 1000))
usePolling(() => { now.value = Math.floor(Date.now() / 1000) }, 1000)

const left = computed(() => (props.view.last_cycle_at ? props.view.last_cycle_at + props.view.cycle_seconds - now.value : null))
const sorting = computed(() => props.view.listened_cycles < props.view.min_listened_cycles)
</script>

<template>
  <p class="m-0 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 rounded-[var(--radius-md)] bg-surface-raised px-3 py-2" data-test="radio-around-status">
    <template v-if="view.listening">
      <span class="text-primary"><span aria-hidden="true" class="mr-2 inline-block h-2 w-2 rounded-full bg-state-running motion-safe:animate-pulse" />
        <span data-test="radio-status-next">{{ left !== null && left > 0 ? radioStatusListening(left) : left === null ? radioStatusListening(view.cycle_seconds) : RADIO_STATUS_READING }}</span></span>
      <span v-if="sorting" class="text-[length:var(--text-small)] text-muted" data-test="radio-status-sorting">{{ radioStatusSorting(view.listened_cycles, view.min_listened_cycles) }}</span>
    </template>
    <template v-else>
      <span class="text-secondary" data-test="radio-status-idle">{{ RADIO_STATUS_IDLE }}</span>
      <RouterLink :to="toRadio()" class="inline-flex min-h-11 items-center text-accent-brand hover:underline" data-test="radio-status-start">{{ RADIO_STATUS_START }}</RouterLink>
    </template>
  </p>
</template>
