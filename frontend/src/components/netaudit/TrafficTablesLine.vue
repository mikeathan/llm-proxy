<script setup lang="ts">
import { computed, onMounted } from "vue"
import { useDestinationData } from "../../composables/netaudit/useDestinationData"
import { toNetworkSettings } from "../../router/routes"
import { TRAFFIC_TABLES_ACTION, TRAFFIC_TABLES_LINE, TRAFFIC_TABLES_OPEN } from "../../constants/netaudit"

// One line under the Traffic tab that says whether the two tables that name internet addresses are held (who runs each, where it is used) and
// links to where they are managed, Settings → Downloaded data. A read that fails says nothing: the line is a pointer, not a status page.
const owners = useDestinationData("owners")
const places = useDestinationData("places")
onMounted(() => { void owners.load(); void places.load() })
const ready = computed(() => owners.state.value !== null && places.state.value !== null)
const hasOwners = computed(() => (owners.state.value?.ranges ?? 0) > 0)
const hasPlaces = computed(() => (places.state.value?.ranges ?? 0) > 0)
</script>

<template>
  <p v-if="ready" class="m-0 flex flex-wrap items-center gap-x-2 text-[length:var(--text-small)] text-muted" data-test="traffic-tables">
    <span data-test="traffic-tables-line">{{ TRAFFIC_TABLES_LINE(hasOwners, hasPlaces) }}</span>
    <RouterLink
      :to="toNetworkSettings('data')"
      :aria-label="TRAFFIC_TABLES_OPEN"
      class="inline-flex min-h-11 items-center text-accent-info-text underline-offset-2 hover:underline sm:min-h-8"
      data-test="traffic-tables-link"
    >{{ TRAFFIC_TABLES_ACTION(hasOwners && hasPlaces) }}</RouterLink>
  </p>
</template>
