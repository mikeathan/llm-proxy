<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import Callout from "../common/feedback/Callout.vue"
import RadioAroundStatus from "./RadioAroundStatus.vue"
import RadioRadarCanvas from "./RadioRadarCanvas.vue"
import RadioDeviceList from "./RadioDeviceList.vue"
import RadioDeviceDetail from "./RadioDeviceDetail.vue"
import RadioReadGuide from "./RadioReadGuide.vue"
import { useRadioLive } from "../../composables/radio/useRadioLive"
import { usePolling } from "../../composables/ui/usePolling"
import { RADIO_AROUND_TITLE, RADIO_RADAR_EMPTY, radioRadarSummary, radioUnusualSummary } from "../../constants/radio"

// Radio · Around you: the radar and the one list of devices, with a status line that says whether it is live. Selecting a dot or a row opens that
// device below. The picture is the server's live answer (memory while listening, the stored days for devices not heard now).
const live = useRadioLive()
const REFRESH_MS = 5000
const selected = ref<string | null>(null)
const view = computed(() => live.view.value)
const everyone = computed(() => view.value?.devices ?? [])
const heard = computed(() => everyone.value.filter((d) => !d.usual))
const mineCount = computed(() => heard.value.filter((d) => d.mine).length)
const unusualCount = computed(() => heard.value.filter((d) => d.unusual).length)
const chosen = computed(() => everyone.value.find((d) => d.id === selected.value) ?? null)
function pick(id: string): void { selected.value = selected.value === id ? null : id }

onMounted(() => { void live.load() })
usePolling(() => { void live.load() }, REFRESH_MS)
</script>

<template>
  <Panel :title="RADIO_AROUND_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-radar">
      <Callout v-if="live.error.value && !view" tone="error" title="Could not read"><span data-test="radio-radar-error">{{ live.error.value }}</span></Callout>
      <template v-else-if="view">
        <RadioAroundStatus :view="view" />
        <p v-if="!everyone.length" class="m-0 text-secondary" data-test="radio-radar-empty">{{ RADIO_RADAR_EMPTY }}</p>
        <template v-else>
          <p class="m-0 text-primary" data-test="radio-radar-summary">{{ radioRadarSummary(heard.length, mineCount) }}<template v-if="unusualCount"><span class="ml-2 text-state-running" data-test="radio-radar-unusual">{{ radioUnusualSummary(unusualCount) }}</span></template></p>
          <div class="grid gap-4 lg:grid-cols-[minmax(0,26rem)_minmax(0,1fr)]">
            <RadioRadarCanvas :devices="everyone" :selected="selected" @pick="pick" />
            <RadioDeviceList :devices="everyone" :selected="selected" @pick="pick" />
          </div>
          <RadioDeviceDetail v-if="chosen" :device="chosen" :listened="view.listened_cycles" :min-listened="view.min_listened_cycles" :busy="live.busy.value" @close="selected = null" @calibrate="live.calibrate" />
          <p v-if="live.error.value" class="m-0 text-state-error" data-test="radio-radar-action-error">{{ live.error.value }}</p>
          <RadioReadGuide :calibrated="view.model.calibrated" :has-usual="everyone.some((d) => d.usual)" />
        </template>
      </template>
    </div>
  </Panel>
</template>
