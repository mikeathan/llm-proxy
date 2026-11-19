<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import Callout from "../common/feedback/Callout.vue"
import RadioAroundStatus from "./RadioAroundStatus.vue"
import RadioAroundControls from "./RadioAroundControls.vue"
import RadioAroundRadar from "./RadioAroundRadar.vue"
import RadioViewRadar from "./RadioViewRadar.vue"
import RadioViewChanges from "./RadioViewChanges.vue"
import RadioViewChannels from "./RadioViewChannels.vue"
import RadioViewNetworks from "./RadioViewNetworks.vue"
import RadioAirwaves from "./RadioAirwaves.vue"
import RadioReadGuide from "./RadioReadGuide.vue"
import { useCardSize } from "../../composables/dashboard/useCardSize"
import { useRadioLive } from "../../composables/radio/useRadioLive"
import { useRadioAround } from "../../composables/radio/useRadioAround"
import { useRadioAirwaves } from "../../composables/radio/useRadioAirwaves"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { usePolling } from "../../composables/ui/usePolling"
import { RADIO_RADAR_EMPTY, radioRadarSummary, radioUnusualSummary } from "../../constants/radio"
import type { RadioAroundToggles } from "../../types/radio"

// Radio · Around you, the dashboard card: one picture of the air (radar, what changed, by channel, networks) picked from a drop-down. The picture is the
// server's live answer (memory while listening, the stored days for devices not heard now). The chips and the channel narrow the radar and the list
// together; a device opens its own page. Small is the summary line; medium adds the radar; large is everything. Naming a device as yours is a
// decision, so it is on the Radio page (Claim your devices), not on the card.
const size = useCardSize()
const live = useRadioLive()
const airwaves = useRadioAirwaves()
const { advanced } = useNetauditViewMode()
const REFRESH_MS = 15000 // the picture changes once a cycle (5 minutes by default)
const MS_PER_SECOND = 1000
const view = computed(() => live.view.value)
const around = useRadioAround(view)
const now = ref(Math.floor(Date.now() / MS_PER_SECOND))
const today = computed(() => new Date(now.value * 1000).toISOString().slice(0, 10))
watch(today, (t) => { around.today.value = t }, { immediate: true })
const everyone = computed(() => around.everyone.value)
const heard = computed(() => everyone.value.filter((d) => !d.usual))
const mineCount = computed(() => heard.value.filter((d) => d.mine).length)
const unusualCount = computed(() => heard.value.filter((d) => d.unusual).length)
const routers = computed(() => airwaves.view.value?.routers ?? [])

onMounted(() => { void live.load(); void airwaves.load() })
usePolling(() => { void live.load() }, REFRESH_MS)
usePolling(() => { now.value = Math.floor(Date.now() / MS_PER_SECOND) }, MS_PER_SECOND)
</script>

<template>
  <div class="flex flex-col gap-3" data-test="radio-radar">
    <Callout v-if="live.error.value && !view" tone="error" title="Could not read"><span data-test="radio-radar-error">{{ live.error.value }}</span></Callout>
    <template v-else-if="view">
      <p v-if="!everyone.length" class="m-0 text-secondary" data-test="radio-radar-empty">{{ RADIO_RADAR_EMPTY }}</p>
      <template v-else>
        <p class="m-0 text-primary" data-test="radio-radar-summary">{{ radioRadarSummary(heard.length, mineCount) }}<template v-if="unusualCount"><span class="ml-2 text-state-running" data-test="radio-radar-unusual">{{ radioUnusualSummary(unusualCount) }}</span></template></p>
        <RadioAroundRadar v-if="size === 'sm'" :around="around" />
        <div v-else-if="size === 'lg'" class="flex flex-col gap-3">
          <RadioAroundStatus :view="view" />
          <RadioAroundControls :mode="around.mode.value" :range="around.toggles.value.range" :spoke="around.toggles.value.spoke" :colours="around.toggles.value.colours" :usual="around.toggles.value.usual" :brief="around.toggles.value.brief" @update:mode="around.mode.value = $event" @toggle="(k: keyof RadioAroundToggles, on: boolean) => around.setToggle(k, on)" />
          <RadioViewRadar v-if="around.mode.value === 'radar'" :around="around" :routers="routers" :now="now" />
          <RadioViewNetworks v-else-if="around.mode.value === 'networks'" />
          <RadioViewChanges v-else-if="around.mode.value === 'changes'" :around="around" :today="today" :now="now" />
          <RadioViewChannels v-else :around="around" :routers="routers" :now="now" />
          <RadioAirwaves v-if="advanced && around.mode.value === 'channels'" />
          <RadioReadGuide :calibrated="view.model.calibrated" :has-usual="everyone.some((d) => d.usual)" />
        </div>
      </template>
    </template>
  </div>
</template>
