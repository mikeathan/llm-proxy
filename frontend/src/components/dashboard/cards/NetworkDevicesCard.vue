<script setup lang="ts">
import { provide, ref } from "vue"
import DevicesCard from "../../netaudit/DevicesCard.vue"
import CaptureWindowCard from "../../netaudit/CaptureWindowCard.vue"
import NearbySection from "../../netaudit/NearbySection.vue"
import SegmentedControl from "../../common/forms/SegmentedControl.vue"
import { useNetauditViewMode } from "../../../composables/netaudit/useNetauditViewMode"
import { useProvideDeviceProblems } from "../../../composables/netaudit/useProvideDeviceProblems"
import { IN_DASHBOARD_CARD } from "../../../constants/dashboard"
import { DEVICES_VIEW_LABEL, DEVICES_VIEW_OPTIONS } from "../../../constants/netaudit"
// Dashboard card · Devices: who has been seen on the network, one row each, with the decision about each (name it, approve it, forget it), and the
// devices nearby that are not on that list, one switch away: both answer "what is out there", so they are one card. The switches that control
// the checks are on the Network page's Modules and Settings tabs. Advanced adds the manual listen on this computer's link.
const { advanced } = useNetauditViewMode()
const view = ref("network")
useProvideDeviceProblems()
// Several panels in one card: each draws its own frame and title (pitfall 74).
provide(IN_DASHBOARD_CARD, false)
</script>

<template>
  <div class="flex flex-col gap-4" data-test="network-devices-card">
    <SegmentedControl v-model="view" :options="DEVICES_VIEW_OPTIONS" :label="DEVICES_VIEW_LABEL" variant="compact" data-test="devices-view" />
    <template v-if="view === 'network'">
      <DevicesCard />
      <CaptureWindowCard v-if="advanced" />
    </template>
    <NearbySection v-else />
  </div>
</template>
