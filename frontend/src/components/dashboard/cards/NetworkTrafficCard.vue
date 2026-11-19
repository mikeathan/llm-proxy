<script setup lang="ts">
import { provide } from "vue"
import TrafficLookPanel from "../../netaudit/TrafficLookPanel.vue"
import TrafficDevices from "../../netaudit/TrafficDevices.vue"
import { useProvideDeviceProblems } from "../../../composables/netaudit/useProvideDeviceProblems"
import { IN_DASHBOARD_CARD } from "../../../constants/dashboard"

// Dashboard card · Traffic by device: where a look at this computer's traffic is taken, and the devices on the network with where each one talks to
// (a map, its places by company). The router that is read for it is set up on the Network page's Settings tab.
useProvideDeviceProblems()
// Several panels in one card: each draws its own frame and title (pitfall 74).
provide(IN_DASHBOARD_CARD, false)
</script>

<template>
  <div class="flex flex-col gap-4">
    <TrafficLookPanel />
    <TrafficDevices />
  </div>
</template>
