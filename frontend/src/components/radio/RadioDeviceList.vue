<script setup lang="ts">
import { computed, ref } from "vue"
import { useRouter } from "vue-router"
import RadioDeviceRow from "./RadioDeviceRow.vue"
import { toRadioDevice } from "../../router/routes"
import { RADIO_LIST_LESS, RADIO_LIST_NONE, RADIO_RADAR_USUAL_HIDE, RADIO_RADAR_USUAL_SHOW, radioListMore, radioUsualMore } from "../../constants/radio"
import type { RadioLiveDevice } from "../../types/radio"

// Radio · the one list of devices, already narrowed by the chips, the channel and the "usual" switch. Closest first; a row picks the device and its
// arrow opens the device's own page.
const props = defineProps<{ devices: RadioLiveDevice[]; selected: string | null; now: number; usualHidden: number; usualOn: boolean }>()
const emit = defineEmits<{ (e: "pick", id: string): void; (e: "usual", on: boolean): void }>()
const PAGE = 12
const showAll = ref(false)
const router = useRouter()
function open(id: string): void { void router.push(toRadioDevice(id)) }
const rows = computed(() => (showAll.value ? props.devices : props.devices.slice(0, PAGE)))
const hidden = computed(() => props.devices.length - rows.value.length)
</script>

<template>
  <div class="flex min-w-0 flex-col gap-1" data-test="radio-device-list">
    <p v-if="!devices.length" class="m-0 text-secondary" data-test="radio-list-none">{{ RADIO_LIST_NONE }}</p>
    <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" data-test="radio-radar-list">
      <li v-for="d in rows" :key="d.id" data-test="radio-radar-row" :data-usual="d.usual ? 1 : 0">
        <RadioDeviceRow :device="d" :selected="selected === d.id" :now="now" @pick="emit('pick', $event)" @open="open" />
      </li>
    </ul>
    <p v-if="usualHidden > 0 || usualOn" class="m-0 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 text-[length:var(--text-small)] text-muted" data-test="radio-radar-usual">
      <span v-if="usualHidden > 0">{{ radioUsualMore(usualHidden) }}</span><span v-else />
      <button type="button" class="min-h-11 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2" :aria-pressed="usualOn" data-test="radio-usual-toggle" @click="emit('usual', !usualOn)">{{ usualOn ? RADIO_RADAR_USUAL_HIDE : RADIO_RADAR_USUAL_SHOW }}</button>
    </p>
    <button v-if="hidden > 0 || (showAll && devices.length > PAGE)" type="button" class="min-h-11 self-start text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2" data-test="radio-list-more" @click="showAll = !showAll">{{ showAll ? RADIO_LIST_LESS : radioListMore(hidden) }}</button>
  </div>
</template>
