<script setup lang="ts">
import { computed } from "vue"
import { useRoute } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackModules from "../components/stacks/StackModules.vue"
import RadioSensors from "../components/radio/RadioSensors.vue"
import RadioListening from "../components/radio/RadioListening.vue"
import RadioAround from "../components/radio/RadioAround.vue"
import RadioOwnerDevices from "../components/radio/RadioOwnerDevices.vue"
import RadioNetworks from "../components/radio/RadioNetworks.vue"
import { RADIO_LISTENING_LABEL, RADIO_SUBTITLE, RADIO_TAB_LABELS, RADIO_TITLE } from "../constants/stacks"
import { toRadio } from "../router/routes"
import { RADIO_TABS } from "../types/routes"
import type { RadioTab } from "../types/routes"
import type { StackTabItem } from "../types/stacks"

// Radio: the third stack. Listening (start, watch, and what is around you) is the page; Sensors says what each radio is for, My devices holds
// the devices and networks that are yours, and Modules is the switchboard.
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
    <StackModules v-if="tab === 'modules'" stack="radio" />
    <RadioSensors v-else-if="tab === 'sensors'" />
    <template v-else-if="tab === 'devices'">
      <RadioOwnerDevices />
      <RadioNetworks />
    </template>
    <template v-else>
      <RadioListening />
      <RadioAround />
    </template>
  </div>
</template>
