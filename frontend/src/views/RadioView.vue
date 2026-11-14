<script setup lang="ts">
import { computed } from "vue"
import { useRoute } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackModules from "../components/stacks/StackModules.vue"
import RadioSensors from "../components/radio/RadioSensors.vue"
import RadioListening from "../components/radio/RadioListening.vue"
import RadioAroundLink from "../components/radio/RadioAroundLink.vue"
import RadioFindings from "../components/radio/RadioFindings.vue"
import RadioNeighbourhood from "../components/radio/RadioNeighbourhood.vue"
import RadioAirwaves from "../components/radio/RadioAirwaves.vue"
import RadioScanPlan from "../components/radio/RadioScanPlan.vue"
import RadioRadar from "../components/radio/RadioRadar.vue"
import SegmentedControl from "../components/common/forms/SegmentedControl.vue"
import { useNetauditViewMode } from "../composables/netaudit/useNetauditViewMode"
import { RADIO_DETAIL_LABEL } from "../constants/radio"
import { VIEW_MODE_OPTIONS } from "../constants/netaudit"
import type { NetauditViewMode } from "../types/netaudit"
import RadioOwnerDevices from "../components/radio/RadioOwnerDevices.vue"
import RadioNetworks from "../components/radio/RadioNetworks.vue"
import { RADIO_LISTENING_LABEL, RADIO_SUBTITLE, RADIO_TAB_LABELS, RADIO_TITLE } from "../constants/stacks"
import { toRadio } from "../router/routes"
import { RADIO_TABS } from "../types/routes"
import type { RadioTab } from "../types/routes"
import type { StackTabItem } from "../types/stacks"

// Radio: the third stack. Listening (start and watch a run) is the page; Around you is the radar, the kinds of device and how the channels are used; My devices holds the
// devices and networks that are yours and where they are; Sensors says what each radio is for; Modules is the switchboard. Simple and Advanced
// are the same remembered choice as on the Network screens.
const { mode, set } = useNetauditViewMode()
const route = useRoute()
const tab = computed<RadioTab | null>(() => {
  const t = route.params.tab
  return typeof t === "string" && (RADIO_TABS as readonly string[]).includes(t) ? (t as RadioTab) : null
})
const tabs: readonly StackTabItem[] = [
  { id: "listening", label: RADIO_LISTENING_LABEL, to: toRadio() },
  ...RADIO_TABS.map((t) => ({ id: t, label: RADIO_TAB_LABELS[t], to: toRadio(t) })),
]
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="RADIO_TITLE" :subtitle="RADIO_SUBTITLE" subtitle-hidden-on-phone />
    <StackTabs :label="RADIO_TITLE" :tabs="tabs" :active="tab ?? 'listening'" />
    <div v-if="tab === 'around' || tab === 'devices'" class="flex justify-end">
      <SegmentedControl :model-value="mode" :options="VIEW_MODE_OPTIONS" :label="RADIO_DETAIL_LABEL" variant="compact" @update:model-value="set($event as NetauditViewMode)" />
    </div>
    <StackModules v-if="tab === 'modules'" stack="radio" />
    <RadioSensors v-else-if="tab === 'sensors'" />
    <template v-else-if="tab === 'around'">
      <RadioFindings />
      <RadioRadar />
      <RadioNeighbourhood />
      <RadioAirwaves />
    </template>
    <template v-else-if="tab === 'devices'">
      <RadioOwnerDevices />
      <RadioNetworks />
    </template>
    <template v-else>
      <RadioListening />
      <RadioScanPlan />
      <RadioAroundLink />
    </template>
  </div>
</template>
