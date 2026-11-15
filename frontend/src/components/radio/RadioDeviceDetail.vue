<script setup lang="ts">
import BaseButton from "../common/buttons/BaseButton.vue"
import RadioTrack from "./RadioTrack.vue"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import {
  RADIO_CLASS_HELP, RADIO_CLASS_WORDS, RADIO_DETAIL_CLOSE, RADIO_DETAIL_FIRST, RADIO_DETAIL_LAST, RADIO_DETAIL_SIGNAL, RADIO_DETAIL_WHY, RADIO_DEVICE_COLUMNS,
  RADIO_JOINED_OWN, RADIO_RADAR_CALIBRATE, RADIO_RANDOM_ADDRESS, RADIO_REASONS, RADIO_SIGNAL_HELP, RADIO_SIGNAL_WORDS, RADIO_TREND_WORDS,
  radioChannelsLine, radioDotLabel, radioHeardLine, radioRangeLine, radioRetryWord, radioWaitingLine, radioWaitingPercent,
} from "../../constants/radio"
import type { RadioLiveDevice } from "../../types/radio"

// Radio · one device, opened from the radar or the list: where it is and how it is moving, why it was sorted into its kind, the distance over time,
// and for the owner's own device the one-metre calibration. Advanced adds the evidence counts. Never an address and never a name a device sent.
defineProps<{ device: RadioLiveDevice; listened: number; minListened: number; busy: boolean }>()
const emit = defineEmits<{ (e: "close"): void; (e: "calibrate", id: string): void }>()
const { advanced } = useNetauditViewMode()
</script>

<template>
  <div class="flex flex-col gap-3 rounded-[var(--radius-md)] bg-surface-raised px-3 py-3" data-test="radio-radar-chosen">
    <div class="flex items-baseline justify-between gap-3">
      <span class="text-primary">{{ radioDotLabel(device) }}: {{ device.usual ? "usually " : "" }}{{ device.dbm }} dBm on channel {{ device.channel }}, {{ radioRangeLine(device.near_m, device.far_m) }}<template v-if="!device.usual">, {{ RADIO_TREND_WORDS[device.trend] }}</template>.</span>
      <button type="button" class="min-h-11 flex-none px-2 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2" data-test="radio-detail-close" @click="emit('close')">{{ RADIO_DETAIL_CLOSE }}</button>
    </div>
    <p v-if="!device.mine" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="radio-detail-kind">
      <span class="text-primary">{{ RADIO_CLASS_WORDS[device.class] }}.</span> {{ RADIO_CLASS_HELP[device.class] }}
    </p>
    <template v-if="device.detail">
      <template v-if="device.detail.why === 'needs_more_listening'">
        <div class="h-1.5 w-full overflow-hidden rounded-full bg-hairline" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="radioWaitingPercent(device.detail.cycles_heard, device.detail.need_heard, listened, minListened)">
          <div class="h-full bg-accent-brand" :style="{ width: radioWaitingPercent(device.detail.cycles_heard, device.detail.need_heard, listened, minListened) + '%' }" />
        </div>
        <span class="text-[length:var(--text-small)] text-muted" data-test="radio-air-waiting">{{ radioWaitingLine(device.detail.cycles_heard, device.detail.need_heard, listened, minListened) }}</span>
      </template>
      <p v-else class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-air-why"><span class="text-secondary">{{ RADIO_DETAIL_WHY }}:</span> {{ RADIO_REASONS[device.detail.why] ?? device.detail.why }}</p>
      <p class="m-0 flex flex-wrap gap-x-4 gap-y-0.5 text-[length:var(--text-small)] text-muted">
        <span data-test="radio-air-signal" :title="RADIO_SIGNAL_HELP[device.detail.signal]">{{ RADIO_DETAIL_SIGNAL }}: {{ RADIO_SIGNAL_WORDS[device.detail.signal] }}</span>
        <span>{{ RADIO_DETAIL_LAST }}: {{ device.detail.last_seen }}</span>
        <span v-if="device.detail.joined_own" class="text-state-success">{{ RADIO_JOINED_OWN }}</span>
      </p>
      <dl v-if="advanced" class="m-0 grid grid-cols-2 gap-x-4 gap-y-0.5 text-[length:var(--text-small)] text-muted sm:grid-cols-4" data-test="radio-detail-evidence">
        <div><dt class="inline">{{ RADIO_DEVICE_COLUMNS.heard }}</dt> <dd class="m-0 inline font-mono text-secondary" data-test="radio-air-heard">{{ radioHeardLine(device.detail.cycles_heard, listened) }}</dd></div>
        <div><dt class="inline">{{ RADIO_DEVICE_COLUMNS.retries }}</dt> <dd class="m-0 inline font-mono text-secondary" data-test="radio-air-retries">{{ radioRetryWord(device.detail.retry_percent) }}</dd></div>
        <div><dt class="inline">{{ RADIO_DEVICE_COLUMNS.channels }}</dt> <dd class="m-0 inline font-mono text-secondary" data-test="radio-air-channels">{{ radioChannelsLine(device.detail.channels) }}</dd></div>
        <div><dt class="inline">{{ RADIO_DETAIL_FIRST }}</dt> <dd class="m-0 inline font-mono text-secondary">{{ device.detail.first_seen }}</dd></div>
      </dl>
      <span v-if="advanced && device.detail.random_address" class="text-[length:var(--text-small)] text-muted">{{ RADIO_RANDOM_ADDRESS }}</span>
    </template>
    <RadioTrack v-if="!device.usual" :id="device.id" />
    <BaseButton v-if="device.mine && !device.usual" variant="secondary" class-name="self-start max-sm:!h-11" :disabled="busy" data-test="radio-calibrate" @click="emit('calibrate', device.id)">{{ RADIO_RADAR_CALIBRATE }}</BaseButton>
  </div>
</template>
