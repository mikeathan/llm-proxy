<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Callout from "../common/feedback/Callout.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import FormField from "../common/forms/FormField.vue"
import StatusTag from "../common/display/StatusTag.vue"
import RadioWindows from "./RadioWindows.vue"
import { useRadioListening } from "../../composables/radio/useRadioListening"
import { useRadioSensors } from "../../composables/radio/useRadioSensors"
import { useRadioScanPlan } from "../../composables/radio/useRadioScanPlan"
import {
  RADIO_ENDED, RADIO_LISTEN_ADAPTER, RADIO_LISTEN_INTRO, RADIO_LISTEN_NO_ADAPTER, RADIO_LISTEN_OFF,
  RADIO_LISTEN_START, RADIO_LISTEN_MANUAL, RADIO_LISTEN_STOP, RADIO_LISTEN_TITLE, radioCycleLine,
  radioFocusLine, radioFramesLine, radioListenLength, radioListeningUntil, RADIO_RESUME_BUTTON, RADIO_RESUME_DISMISS, RADIO_RESUME_TITLE, radioResumeLine,
} from "../../constants/radio"

// Radio · Listening: choose an adapter and a time, confirm the passphrase once, and watch the windows come in. The run is the stack's; a
// session in the core is what lets it ask the helper, and stopping ends both. Only counts and words are shown here, never an address.
const l = useRadioListening()
const s = useRadioSensors()
const p = useRadioScanPlan()
const adapter = ref("")

const adapters = computed(() => (s.view.value?.sensors ?? []).filter((x) => x.kind === "wifi" && x.eligible))
const adapterOptions = computed(() => adapters.value.map((x) => ({ value: x.id, label: x.id })))
const lengthLine = computed(() => (p.scan.value ? radioListenLength(p.scan.value.hours) : ""))
const off = computed(() => !!s.view.value && !s.view.value.enabled)
const running = computed(() => !!l.state.value?.listening)
const ended = computed(() => (l.state.value && !l.state.value.listening && l.state.value.ended ? RADIO_ENDED[l.state.value.ended] : null))
const until = computed(() => (l.state.value?.until ? new Date(l.state.value.until).toLocaleString() : ""))
// Where in the cycle the run is: the position of the last window's channel in the plan (a cycle visits each channel once, in order).
const channels = computed(() => l.state.value?.plan.channels ?? [])
const heardInCycle = computed(() => {
  const windows = l.state.value?.windows ?? []
  const last = windows[windows.length - 1]
  return last ? channels.value.indexOf(last.channel) + 1 : 0
})
const resumeOffer = computed(() => (!running.value && !off.value ? l.state.value?.resume ?? null : null))
const resumeLine = computed(() => (resumeOffer.value ? radioResumeLine(resumeOffer.value.why, resumeOffer.value.hours_left, new Date(resumeOffer.value.until).toLocaleString()) : ""))
const canStart = computed(() => !off.value && !running.value && !l.busy.value && !!(adapter.value || adapters.value.length))

function start(): void { void l.start(adapter.value || adapters.value[0]?.id || "") }

onMounted(async () => {
  await Promise.all([l.refresh(), s.load(), p.load()])
  adapter.value = adapters.value[0]?.id ?? ""
})
</script>

<template>
  <Panel :title="RADIO_LISTEN_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-listening">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_LISTEN_INTRO }}</p>

      <Callout v-if="off" tone="warning" title="The Radio stack is off"><span data-test="radio-listen-off">{{ RADIO_LISTEN_OFF }}</span></Callout>
      <Callout v-else-if="s.view.value && !adapters.length" tone="info" title="No adapter"><span data-test="radio-listen-noadapter">{{ RADIO_LISTEN_NO_ADAPTER }}</span></Callout>

      <Callout v-if="resumeOffer" tone="warning" :title="RADIO_RESUME_TITLE">
        <span data-test="radio-resume-line">{{ resumeLine }}</span>
        <span class="mt-2 flex flex-wrap gap-2">
          <BaseButton variant="primary" class-name="max-sm:!h-11" :disabled="l.busy.value" data-test="radio-resume" @click="l.resume">{{ RADIO_RESUME_BUTTON }}</BaseButton>
          <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="l.busy.value" data-test="radio-resume-dismiss" @click="l.stop">{{ RADIO_RESUME_DISMISS }}</BaseButton>
        </span>
      </Callout>

      <div v-if="running" class="flex flex-col gap-2" data-test="radio-listen-running">
        <p class="m-0 flex flex-wrap items-center gap-2 text-primary">
          <StatusTag state="running" label="Listening" />
          <span data-test="radio-listen-until">{{ radioListeningUntil(until) }}</span>
        </p>
        <p class="m-0 text-[length:var(--text-small)] text-secondary" data-test="radio-listen-progress">{{ radioCycleLine(l.state.value?.cycles ?? 0, heardInCycle, channels.length) }} {{ radioFramesLine(l.state.value?.frames ?? 0) }} {{ radioFocusLine(l.state.value?.focus_windows ?? 0, !!l.state.value?.focus_off) }}</p>
        <div><BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="l.busy.value" data-test="radio-listen-stop" @click="l.stop">{{ RADIO_LISTEN_STOP }}</BaseButton></div>
      </div>

      <div v-else-if="!off && adapters.length" class="flex flex-wrap items-end gap-3">
        <FormField :label="RADIO_LISTEN_ADAPTER"><template #default><SelectInput v-model="adapter" :options="adapterOptions" :label="RADIO_LISTEN_ADAPTER" /></template></FormField>
        <BaseButton variant="primary" class-name="max-sm:!h-11" :disabled="!canStart" data-test="radio-listen-start" @click="start">{{ RADIO_LISTEN_START }}</BaseButton>
      </div>
      <p v-if="!running && !off && adapters.length && lengthLine" class="m-0 text-[length:var(--text-small)] text-primary" data-test="radio-listen-length">{{ lengthLine }}</p>
      <p v-if="!running && !off && adapters.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-listen-manual">{{ RADIO_LISTEN_MANUAL }}</p>

      <Callout v-if="l.error.value" tone="error" title="Listening did not start"><span data-test="radio-listen-error">{{ l.error.value }}</span></Callout>
      <Callout v-if="ended" tone="info" :title="ended.title"><span data-test="radio-listen-ended">{{ ended.next }}</span><span v-if="l.state.value?.last_error" class="block text-muted" data-test="radio-listen-last-error">{{ l.state.value.last_error }}</span></Callout>

      <RadioWindows v-if="l.state.value && (l.state.value.windows.length || running)" :windows="l.state.value.windows" />
    </div>
  </Panel>
</template>
