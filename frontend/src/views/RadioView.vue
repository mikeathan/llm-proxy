<script setup lang="ts">
import { computed, ref } from "vue"
import { useRoute } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackModules from "../components/stacks/StackModules.vue"
import StackBoardLink from "../components/stacks/StackBoardLink.vue"
import RadioListening from "../components/radio/RadioListening.vue"
import RadioScanPlan from "../components/radio/RadioScanPlan.vue"
import RadioKeeping from "../components/radio/RadioKeeping.vue"
import { RADIO_LISTENING_LABEL, RADIO_SUBTITLE, RADIO_TAB_LABELS, RADIO_TITLE } from "../constants/stacks"
import { toRadio } from "../router/routes"
import { RADIO_TABS } from "../types/routes"
import type { RadioTab } from "../types/routes"
import type { StackTabItem } from "../types/stacks"

// Radio: the third stack. Listening (start and watch a run, and how it is set up) is the page, because starting it needs the passphrase; Modules is the
// switchboard. What each radio is for, the radar, your devices (and claiming, adding and removing them), your networks, what changed and
// how the channels are used are cards on the dashboard's Radio board, which the header links to; the old /radio/devices address opens its card.
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
    <StackModules v-if="tab === 'modules'" stack="radio" />
    <template v-else>
      <RadioListening :key="planSaves" />
      <RadioScanPlan @saved="planSaves++" />
      <RadioKeeping />
    </template>
  </div>
</template>
