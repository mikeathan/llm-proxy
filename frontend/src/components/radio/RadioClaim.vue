<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import RadioAskCard from "./RadioAskCard.vue"
import RadioFindMine from "./RadioFindMine.vue"
import { useRadioLive } from "../../composables/radio/useRadioLive"
import { useRadioAround } from "../../composables/radio/useRadioAround"
import { usePolling } from "../../composables/ui/usePolling"
import { RADIO_CLAIM_INTRO, RADIO_CLAIM_TITLE } from "../../constants/radio"

// Radio · Claim your devices: the one place to tell the stack which devices are yours. Find my device is the way to start (hold the device
// next to the computer and name the strongest one); Help it learn asks about the few devices an answer would teach the sorting most
// about. Both record ownership only. They used to sit under the radar; they are decisions, so they live on the Radio page and the radar
// on the dashboard only shows what was decided.
const live = useRadioLive()
const around = useRadioAround(computed(() => live.view.value))
const REFRESH_MS = 15000
const MS_PER_SECOND = 1000
const now = ref(Math.floor(Date.now() / MS_PER_SECOND))
const everyone = computed(() => around.everyone.value)

async function mark(id: string, label: string): Promise<void> {
  await live.markMine(id, label)
}

onMounted(() => { void live.load() })
usePolling(() => { void live.load() }, REFRESH_MS)
usePolling(() => { now.value = Math.floor(Date.now() / MS_PER_SECOND) }, MS_PER_SECOND)
</script>

<template>
  <Panel :title="RADIO_CLAIM_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-claim">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_CLAIM_INTRO }}</p>
      <RadioFindMine :devices="everyone" :busy="live.busy.value" :now="now" @mark="mark" />
      <RadioAskCard @answered="live.load()" />
      <p v-if="live.error.value" class="m-0 text-state-error" data-test="radio-claim-error">{{ live.error.value }}</p>
    </div>
  </Panel>
</template>
