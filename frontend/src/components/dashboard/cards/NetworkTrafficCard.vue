<script setup lang="ts">
import { onMounted, provide } from "vue"
import TrafficLookPanel from "../../netaudit/TrafficLookPanel.vue"
import TrafficDevices from "../../netaudit/TrafficDevices.vue"
import TrafficTraceCard from "../../netaudit/TrafficTraceCard.vue"
import { useProvideDeviceProblems } from "../../../composables/netaudit/useProvideDeviceProblems"
import { useRouteTrace } from "../../../composables/netaudit/useRouteTrace"
import { usePolling } from "../../../composables/ui/usePolling"
import { IN_DASHBOARD_CARD } from "../../../constants/dashboard"
import { ROUTE_POLL_MS } from "../../../constants/netaudit"
// Dashboard card · Traffic: where a look at this computer's traffic is taken, the devices on the network with where each one talks to (a map, its
// places by company), and below them the trace of the routers between this computer and one internet address: both answer "where does the traffic
// go". The router that is read for it is set up on the Network page's Settings tab. A trace that is running fills in step by step, so it is read
// every few seconds while the card is on screen.
useProvideDeviceProblems()
const routes = useRouteTrace()
onMounted(() => void routes.load())
usePolling(() => void routes.refresh(), ROUTE_POLL_MS)
// Several panels in one card: each draws its own frame and title (pitfall 74).
provide(IN_DASHBOARD_CARD, false)
</script>

<template>
  <div class="flex flex-col gap-4" data-test="network-traffic-card">
    <TrafficLookPanel />
    <TrafficDevices />
    <TrafficTraceCard />
  </div>
</template>
