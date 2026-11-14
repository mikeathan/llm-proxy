<script setup lang="ts">
import { computed, onMounted } from "vue"
import { useRadioAir } from "../../composables/radio/useRadioAir"
import { RADIO_AROUND_EMPTY, RADIO_NEIGH_LINK, RADIO_TAB_LINK_AROUND, radioAroundTotal } from "../../constants/radio"
import { toRadio } from "../../router/routes"

// Radio · Listening: one line about who is around, with the way to the Neighbourhood tab where each kind opens onto its devices.
const a = useRadioAir()
const total = computed(() => a.air.value?.total ?? 0)
onMounted(() => { void a.load() })
</script>

<template>
  <p v-if="a.air.value" class="m-0 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 rounded-[var(--radius-md)] bg-surface-raised px-4 py-3" data-test="radio-around-link">
    <span class="text-primary" data-test="radio-around-total">{{ total || a.air.value.listened_cycles ? radioAroundTotal(total) : RADIO_AROUND_EMPTY }}</span>
    <RouterLink :to="toRadio('around')" class="inline-flex min-h-11 items-center text-accent-brand hover:underline" :aria-label="RADIO_TAB_LINK_AROUND">{{ RADIO_NEIGH_LINK }}</RouterLink>
  </p>
</template>
