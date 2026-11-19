<script setup lang="ts">
import { onMounted } from "vue"
import TrafficTraceCard from "../../netaudit/TrafficTraceCard.vue"
import { usePolling } from "../../../composables/ui/usePolling"
import { useRouteTrace } from "../../../composables/netaudit/useRouteTrace"
import { ROUTE_POLL_MS } from "../../../constants/netaudit"

// Dashboard card · Trace a route: the routers between this computer and an internet address. The server keeps the traces; one that is running fills
// in step by step, so it is read every few seconds while the card is on screen.
const routes = useRouteTrace()
onMounted(() => void routes.load())
usePolling(() => void routes.refresh(), ROUTE_POLL_MS)
</script>

<template>
  <TrafficTraceCard />
</template>
