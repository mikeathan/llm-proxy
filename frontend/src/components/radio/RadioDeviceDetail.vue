<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import RadioTrack from "./RadioTrack.vue"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import {
  RADIO_CLASS_HELP, RADIO_CLASS_WORDS, RADIO_DETAIL_ADDRESS, RADIO_DETAIL_ADDRESS_RANDOM, RADIO_DETAIL_ADDRESS_VALUE, RADIO_DETAIL_CLOSE, RADIO_DETAIL_FIRST,
  RADIO_DETAIL_HISTORY, RADIO_DETAIL_LAST, RADIO_DETAIL_MAKER, RADIO_DETAIL_NETWORK, RADIO_DETAIL_NETWORK_NOTE, RADIO_DETAIL_SIGNAL, RADIO_DETAIL_WHERE,
  RADIO_DETAIL_WHY, RADIO_DETAIL_WIFI, RADIO_DEVICE_COLUMNS, RADIO_JOINED_OWN, RADIO_MAKER_RANDOM_HELP, RADIO_MARK_BUTTON, RADIO_MARK_LABEL, RADIO_MARK_NO_NAME,
  RADIO_MARK_NOTE, RADIO_MARK_PLACEHOLDER, RADIO_MARK_SAVE, RADIO_RADAR_CALIBRATE, RADIO_REASONS, RADIO_RING_WORDS, RADIO_SIGNAL_HELP, RADIO_SIGNAL_WORDS,
  RADIO_TREND_WORDS, radioChannelBand, radioChannelsLine, radioFirstHeardRun, radioHeardCycles, radioHeardLine, radioRangeLine, radioRetryWord,
  radioUsualLine, radioWaitingLine, radioWaitingPercent, radioWifiWord,
} from "../../constants/radio"
import { deviceTitle, heardAgoWord } from "../../utils/radio/describe"
import { conclusionOf } from "../../utils/radio/identity"
import { RADIO_DETAIL_OPEN } from "../../constants/radioIdentity"
import { toRadioDevice } from "../../router/routes"
import type { RadioLiveDevice } from "../../types/radio"

// Radio · one device, opened from the radar or the list: what it is as far as its frames say (maker, Wi-Fi generation, network name, fixed or random
// address), where it is and how it is moving, how it compares with its own history, why it was sorted into its kind, the distance over time, and a way
// to name it as yours. For the owner's own device: the one-metre calibration. Advanced adds the evidence counts. Never an address.
const props = defineProps<{ device: RadioLiveDevice; listened: number; minListened: number; busy: boolean; now: number }>()
const emit = defineEmits<{ (e: "close"): void; (e: "calibrate", id: string): void; (e: "mark", id: string, label: string): void }>()
const { advanced } = useNetauditViewMode()
const naming = ref(false)
const label = ref("")
const missingName = ref(false)
const d = computed(() => props.device.detail)
const runSeconds = computed(() => (props.device.heard_since ? Math.max(0, props.now - props.device.heard_since) : null))

function save(): void {
  const name = label.value.trim()
  missingName.value = !name
  if (!name) return
  emit("mark", props.device.id, name)
  naming.value = false
  label.value = ""
}
</script>

<template>
  <div class="flex flex-col gap-3 rounded-[var(--radius-md)] bg-surface-raised px-3 py-3" data-test="radio-radar-chosen">
    <div class="flex items-baseline justify-between gap-3">
      <span class="text-primary">{{ deviceTitle(device) }} <span class="font-mono text-[length:var(--text-small)] text-faint">{{ device.id }}</span>: {{ device.usual ? "usually " : "" }}{{ device.dbm }} dBm on {{ radioChannelBand(device.channel) }}, {{ radioRangeLine(device.near_m, device.far_m) }}<template v-if="!device.usual">, {{ RADIO_TREND_WORDS[device.trend] }}</template>.</span>
      <span class="flex flex-none items-baseline gap-1">
        <RouterLink :to="toRadioDevice(device.id)" class="inline-flex min-h-11 items-center px-2 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2 sm:min-h-8" data-test="radio-detail-open">{{ RADIO_DETAIL_OPEN }}</RouterLink>
        <button type="button" class="min-h-11 flex-none px-2 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2" data-test="radio-detail-close" @click="emit('close')">{{ RADIO_DETAIL_CLOSE }}</button>
      </span>
    </div>
    <p class="m-0 text-secondary" data-test="radio-detail-conclusion">{{ conclusionOf(device) }}</p>
    <p v-if="!device.mine" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="radio-detail-kind">
      <span class="text-primary">{{ RADIO_CLASS_WORDS[device.class] }}.</span> {{ RADIO_CLASS_HELP[device.class] }}
    </p>
    <dl class="m-0 grid grid-cols-1 gap-x-6 gap-y-1 text-[length:var(--text-small)] sm:grid-cols-2" data-test="radio-detail-facts">
      <div v-if="d?.maker" class="flex gap-2"><dt class="text-muted">{{ RADIO_DETAIL_MAKER }}</dt><dd class="m-0 text-primary" data-test="radio-detail-maker">{{ d.maker }}</dd></div>
      <div v-if="device.network" class="flex gap-2"><dt class="text-muted">{{ RADIO_DETAIL_NETWORK }}</dt><dd class="m-0 text-primary" data-test="radio-detail-network" :title="RADIO_DETAIL_NETWORK_NOTE">{{ device.network }}</dd></div>
      <div v-if="d?.wifi_generation" class="flex gap-2"><dt class="text-muted">{{ RADIO_DETAIL_WIFI }}</dt><dd class="m-0 text-primary">{{ radioWifiWord(d.wifi_generation) }}</dd></div>
      <div v-if="d" class="flex gap-2"><dt class="text-muted">{{ RADIO_DETAIL_ADDRESS }}</dt><dd class="m-0 text-primary" :title="d.random_address ? RADIO_MAKER_RANDOM_HELP : ''" data-test="radio-detail-address">{{ d.random_address ? RADIO_DETAIL_ADDRESS_RANDOM : RADIO_DETAIL_ADDRESS_VALUE }}</dd></div>
      <div class="flex gap-2"><dt class="text-muted">{{ RADIO_DETAIL_WHERE }}</dt><dd class="m-0 text-primary">{{ RADIO_RING_WORDS[device.ring] }}</dd></div>
      <div v-if="d" class="flex gap-2"><dt class="text-muted">{{ RADIO_DETAIL_SIGNAL }}</dt><dd class="m-0 text-primary" data-test="radio-air-signal" :title="RADIO_SIGNAL_HELP[d.signal]">{{ RADIO_SIGNAL_WORDS[d.signal] }}</dd></div>
      <div class="flex gap-2"><dt class="text-muted">{{ RADIO_DETAIL_LAST }}</dt><dd class="m-0 text-primary" data-test="radio-detail-last">{{ heardAgoWord(device, now) || d?.last_seen }}</dd></div>
      <div v-if="d" class="flex gap-2"><dt class="text-muted">{{ RADIO_DETAIL_FIRST }}</dt><dd class="m-0 text-primary">{{ d.first_seen }}<template v-if="runSeconds !== null && device.heard_since"> · {{ radioFirstHeardRun(runSeconds) }}</template></dd></div>
      <div v-if="d || device.usual_dbm" class="flex gap-2 sm:col-span-2"><dt class="text-muted">{{ RADIO_DETAIL_HISTORY }}</dt>
        <dd class="m-0 text-primary" data-test="radio-detail-history"><template v-if="d">{{ radioHeardCycles(d.cycles_heard) }}. </template><template v-if="device.usual_dbm">{{ radioUsualLine(device.usual_dbm, device.dbm) }}</template></dd></div>
      <div v-if="d?.joined_own" class="text-state-success sm:col-span-2">{{ RADIO_JOINED_OWN }}</div>
    </dl>
    <template v-if="d">
      <template v-if="d.why === 'needs_more_listening'">
        <div class="h-1.5 w-full overflow-hidden rounded-full bg-hairline" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="radioWaitingPercent(d.cycles_heard, d.need_heard, listened, minListened)">
          <div class="h-full bg-accent-brand" :style="{ width: radioWaitingPercent(d.cycles_heard, d.need_heard, listened, minListened) + '%' }" />
        </div>
        <span class="text-[length:var(--text-small)] text-muted" data-test="radio-air-waiting">{{ radioWaitingLine(d.cycles_heard, d.need_heard, listened, minListened) }}</span>
      </template>
      <p v-else class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-air-why"><span class="text-secondary">{{ RADIO_DETAIL_WHY }}:</span> {{ RADIO_REASONS[d.why] ?? d.why }}</p>
      <dl v-if="advanced" class="m-0 grid grid-cols-2 gap-x-4 gap-y-0.5 text-[length:var(--text-small)] text-muted sm:grid-cols-3" data-test="radio-detail-evidence">
        <div><dt class="inline">{{ RADIO_DEVICE_COLUMNS.heard }}</dt> <dd class="m-0 inline font-mono text-secondary" data-test="radio-air-heard">{{ radioHeardLine(d.cycles_heard, listened) }}</dd></div>
        <div><dt class="inline">{{ RADIO_DEVICE_COLUMNS.retries }}</dt> <dd class="m-0 inline font-mono text-secondary" data-test="radio-air-retries">{{ radioRetryWord(d.retry_percent) }}</dd></div>
        <div><dt class="inline">{{ RADIO_DEVICE_COLUMNS.channels }}</dt> <dd class="m-0 inline font-mono text-secondary" data-test="radio-air-channels">{{ radioChannelsLine(d.channels) }}</dd></div>
      </dl>
    </template>
    <RadioTrack v-if="!device.usual" :id="device.id" />
    <div v-if="!device.mine" class="flex flex-col gap-2" data-test="radio-mark">
      <BaseButton v-if="!naming" variant="secondary" class-name="self-start max-sm:!h-11" data-test="radio-mark-open" @click="naming = true">{{ RADIO_MARK_BUTTON }}</BaseButton>
      <form v-else class="flex flex-col gap-2" @submit.prevent="save">
        <label class="flex flex-col gap-1 text-[length:var(--text-small)] text-secondary">{{ RADIO_MARK_LABEL }}
          <input v-model="label" type="text" maxlength="60" :placeholder="RADIO_MARK_PLACEHOLDER" class="h-8 w-full min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2 max-sm:h-11 sm:max-w-xs" data-test="radio-mark-input" />
        </label>
        <p v-if="missingName" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="radio-mark-missing">{{ RADIO_MARK_NO_NAME }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_MARK_NOTE }}</p>
        <BaseButton type="submit" variant="primary" class-name="self-start max-sm:!h-11" :disabled="busy" data-test="radio-mark-save">{{ RADIO_MARK_SAVE }}</BaseButton>
      </form>
    </div>
    <BaseButton v-if="device.mine && !device.usual" variant="secondary" class-name="self-start max-sm:!h-11" :disabled="busy" data-test="radio-calibrate" @click="emit('calibrate', device.id)">{{ RADIO_RADAR_CALIBRATE }}</BaseButton>
  </div>
</template>
