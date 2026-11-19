<script setup lang="ts">
import { computed, ref } from "vue"
import { useRoute } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackModules from "../components/stacks/StackModules.vue"
import StackBoardLink from "../components/stacks/StackBoardLink.vue"
import RadioSensors from "../components/radio/RadioSensors.vue"
import RadioListening from "../components/radio/RadioListening.vue"
import RadioScanPlan from "../components/radio/RadioScanPlan.vue"
import RadioKeeping from "../components/radio/RadioKeeping.vue"
import RadioClaim from "../components/radio/RadioClaim.vue"
import SegmentedControl from "../components/common/forms/SegmentedControl.vue"
import { useNetauditViewMode } from "../composables/netaudit/useNetauditViewMode"
import { RADIO_DETAIL_LABEL } from "../constants/radio"
import { VIEW_MODE_OPTIONS } from "../constants/netaudit"
import type { NetauditViewMode } from "../types/netaudit"
import RadioMyDevices from "../components/radio/RadioMyDevices.vue"
import RadioNetworks from "../components/radio/RadioNetworks.vue"
import { RADIO_LISTENING_LABEL, RADIO_SUBTITLE, RADIO_TAB_LABELS, RADIO_TITLE } from "../constants/stacks"
import { toRadio } from "../router/routes"
import { RADIO_TABS } from "../types/routes"
import type { RadioTab } from "../types/routes"
import type { StackTabItem } from "../types/stacks"

// Radio: the third stack. Listening (start and watch a run) is the page; My devices holds the devices and networks that are yours, the one place
// to claim a device, and where they are; Sensors says what each radio is for; Modules is the switchboard. The radar, what changed and how the
// channels are used are on the dashboard's Radio board, which the header links to. Simple and Advanced are the same remembered choice as on
// the Network screens.
const { mode, set } = useNetauditViewMode()
const route = useRoute()
// Counts the saves of the scan plan: the Listening panel is rebuilt on each, so its "runs for" line never shows an old length.
const planSaves = ref(0)
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
    <StackBoardLink stack="radio" />
    <div v-if="tab === 'devices'" class="flex justify-end">
      <SegmentedControl :model-value="mode" :options="VIEW_MODE_OPTIONS" :label="RADIO_DETAIL_LABEL" variant="compact" @update:model-value="set($event as NetauditViewMode)" />
    </div>
    <StackModules v-if="tab === 'modules'" stack="radio" />
    <RadioSensors v-else-if="tab === 'sensors'" />
    <template v-else-if="tab === 'devices'">
      <RadioClaim />
      <RadioMyDevices />
      <RadioNetworks />
    </template>
    <template v-else>
      <RadioListening :key="planSaves" />
      <RadioScanPlan @saved="planSaves++" />
      <RadioKeeping />
    </template>
  </div>
</template>
