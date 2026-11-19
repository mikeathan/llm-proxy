<script setup lang="ts">
import { computed, onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import Callout from "../common/feedback/Callout.vue"
import RadioMyDeviceRow from "./RadioMyDeviceRow.vue"
import { useRadioDevices } from "../../composables/radio/useRadioDevices"
import { useRadioPresence } from "../../composables/radio/useRadioPresence"
import { useRadioLive } from "../../composables/radio/useRadioLive"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { duplicateNames, myDeviceRows, rowState } from "../../utils/radio/myDevices"
import { toRadio } from "../../router/routes"
import {
  RADIO_DEVICES_EMPTY, RADIO_DEVICES_INTRO, RADIO_DEVICES_TITLE, RADIO_DUP_TITLE, RADIO_MY_START, radioDupLine, radioMySummary,
} from "../../constants/radio"

// Radio · My devices (the dashboard card): your devices, home ones first, each a row that opens its page. Three reads make the rows: your
// devices, where they were hour by hour, and the live picture (the router box and "moving now"; a failed live read only leaves those out). Two
// names alike are shown as a problem with its fix. It only reads: marking, adding by address and removing are on the Radio page's My devices tab.
const d = useRadioDevices()
const p = useRadioPresence()
const live = useRadioLive()
const { advanced } = useNetauditViewMode()

const rows = computed(() => myDeviceRows(d.devices.value ?? [], p.devices.value, live.view.value?.devices ?? []))
const listening = computed(() => !!live.view.value?.listening)
// Home is heard here now or seen on the network now (ADR-0061); either listener is enough to say who is home.
const here = computed(() => rows.value.filter((r) => rowState(r) === "here" || rowState(r) === "on_network").length)
const known = computed(() => listening.value || rows.value.some((r) => r.onNetwork))
const dups = computed(() => duplicateNames(rows.value))

onMounted(() => { void d.load(); void p.load(); void live.load() })
</script>

<template>
  <Panel :title="RADIO_DEVICES_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-devices">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_DEVICES_INTRO }}</p>
      <Callout v-if="d.error.value" tone="error" title="The devices could not be read"><span data-test="radio-devices-error">{{ d.error.value }}</span></Callout>
      <template v-if="rows.length">
        <p class="m-0 flex flex-wrap items-center gap-x-3 gap-y-1 text-primary" data-test="radio-my-summary">
          <span>{{ radioMySummary(here, rows.length, known) }}</span>
          <RouterLink v-if="live.view.value && !listening" :to="toRadio()" class="text-[length:var(--text-small)] text-accent-brand hover:underline" data-test="radio-my-start">{{ RADIO_MY_START }}</RouterLink>
        </p>
        <Callout v-if="dups.length" tone="warning" :title="RADIO_DUP_TITLE">
          <ul class="m-0 flex list-none flex-col gap-1 p-0" data-test="radio-my-dups">
            <li v-for="x in dups" :key="x.label">{{ radioDupLine(x.label, x.count) }}</li>
          </ul>
        </Callout>
        <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" data-test="radio-my-list">
          <RadioMyDeviceRow v-for="r in rows" :key="r.id" :row="r" :advanced="advanced" />
        </ul>
      </template>
      <p v-else-if="d.devices.value" class="m-0 text-secondary" data-test="radio-devices-empty">{{ RADIO_DEVICES_EMPTY }}</p>
    </div>
  </Panel>
</template>
