<script setup lang="ts">
import { provide } from "vue"
import DevicesCard from "../../netaudit/DevicesCard.vue"
import CaptureWindowCard from "../../netaudit/CaptureWindowCard.vue"
import { useNetauditViewMode } from "../../../composables/netaudit/useNetauditViewMode"
import { useProvideDeviceProblems } from "../../../composables/netaudit/useProvideDeviceProblems"
import { IN_DASHBOARD_CARD } from "../../../constants/dashboard"

// Dashboard card · Devices: who has been seen on the network, one row each, with the decision about each (name it, approve it, forget it). The
// switches that control the check are on the Network page's Modules and Settings tabs. Advanced adds the manual listen on this computer's link.
const { advanced } = useNetauditViewMode()
useProvideDeviceProblems()
// Several panels in one card: each draws its own frame and title (pitfall 74).
provide(IN_DASHBOARD_CARD, false)
</script>

<template>
  <div class="flex flex-col gap-4" data-test="network-devices-card">
    <DevicesCard />
    <CaptureWindowCard v-if="advanced" />
  </div>
</template>
