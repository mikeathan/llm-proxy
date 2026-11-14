<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Callout from "../common/feedback/Callout.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import FormField from "../common/forms/FormField.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { useRadioListening } from "../../composables/radio/useRadioListening"
import { useRadioSensors } from "../../composables/radio/useRadioSensors"
import {
  RADIO_ENDED, RADIO_LISTEN_ADAPTER, RADIO_LISTEN_HOURS, RADIO_LISTEN_HOUR_CHOICES, RADIO_LISTEN_INTRO, RADIO_LISTEN_NO_ADAPTER, RADIO_LISTEN_OFF,
  RADIO_LISTEN_START, RADIO_LISTEN_STOP, RADIO_LISTEN_TITLE, RADIO_WINDOWS_TITLE, RADIO_WINDOW_OFF_CHANNEL, RADIO_WINDOW_ON_CHANNEL, radioCycleLine,
  radioHoursLabel, radioListeningUntil,
} from "../../constants/radio"

// Radio · Listening: choose an adapter and a time, confirm the passphrase once, and watch the windows come in. The run is the stack's; a
// session in the core is what lets it ask the helper, and stopping ends both. Only counts and words are shown here, never an address.
const l = useRadioListening()
const s = useRadioSensors()
const adapter = ref("")
const hours = ref("1")

const adapters = computed(() => (s.view.value?.sensors ?? []).filter((x) => x.kind === "wifi" && x.eligible))
const adapterOptions = computed(() => adapters.value.map((x) => ({ value: x.id, label: x.id })))
const hourOptions = RADIO_LISTEN_HOUR_CHOICES.map((h) => ({ value: String(h), label: radioHoursLabel(h) }))
const off = computed(() => !!s.view.value && !s.view.value.enabled)
const running = computed(() => !!l.state.value?.listening)
const ended = computed(() => (l.state.value && !l.state.value.listening && l.state.value.ended ? RADIO_ENDED[l.state.value.ended] : null))
const until = computed(() => (l.state.value?.until ? new Date(l.state.value.until).toLocaleString() : ""))
const canStart = computed(() => !off.value && !running.value && !l.busy.value && !!(adapter.value || adapters.value.length))

function start(): void { void l.start(adapter.value || adapters.value[0]?.id || "", Number(hours.value)) }

onMounted(async () => {
  await Promise.all([l.refresh(), s.load()])
  adapter.value = adapters.value[0]?.id ?? ""
})
</script>

<template>
  <Panel :title="RADIO_LISTEN_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-listening">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_LISTEN_INTRO }}</p>

      <Callout v-if="off" tone="warning" title="The Radio stack is off"><span data-test="radio-listen-off">{{ RADIO_LISTEN_OFF }}</span></Callout>
      <Callout v-else-if="s.view.value && !adapters.length" tone="info" title="No adapter"><span data-test="radio-listen-noadapter">{{ RADIO_LISTEN_NO_ADAPTER }}</span></Callout>

      <div v-if="running" class="flex flex-col gap-2" data-test="radio-listen-running">
        <p class="m-0 flex flex-wrap items-center gap-2 text-primary">
          <StatusTag state="running" label="Listening" />
          <span data-test="radio-listen-until">{{ radioListeningUntil(until) }}</span>
        </p>
        <p class="m-0 text-[length:var(--text-small)] text-secondary">{{ radioCycleLine(l.state.value?.cycles ?? 0, l.state.value?.frames ?? 0) }}</p>
        <div><BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="l.busy.value" data-test="radio-listen-stop" @click="l.stop">{{ RADIO_LISTEN_STOP }}</BaseButton></div>
      </div>

      <div v-else-if="!off && adapters.length" class="flex flex-wrap items-end gap-3">
        <FormField :label="RADIO_LISTEN_ADAPTER"><template #default><SelectInput v-model="adapter" :options="adapterOptions" :label="RADIO_LISTEN_ADAPTER" /></template></FormField>
        <FormField :label="RADIO_LISTEN_HOURS"><template #default><SelectInput v-model="hours" :options="hourOptions" :label="RADIO_LISTEN_HOURS" /></template></FormField>
        <BaseButton variant="primary" class-name="max-sm:!h-11" :disabled="!canStart" data-test="radio-listen-start" @click="start">{{ RADIO_LISTEN_START }}</BaseButton>
      </div>

      <Callout v-if="l.error.value" tone="error" title="Listening did not start"><span data-test="radio-listen-error">{{ l.error.value }}</span></Callout>
      <Callout v-if="ended" tone="info" :title="ended.title"><span data-test="radio-listen-ended">{{ ended.next }}</span><span v-if="l.state.value?.last_error" class="block text-muted" data-test="radio-listen-last-error">{{ l.state.value.last_error }}</span></Callout>

      <div v-if="l.state.value?.windows.length" class="flex flex-col gap-1">
        <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ RADIO_WINDOWS_TITLE }}</h3>
        <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
          <li v-for="w in [...l.state.value.windows].reverse().slice(0, 8)" :key="w.at + w.channel" class="flex flex-wrap items-center gap-x-3 gap-y-0.5 py-1.5 text-[length:var(--text-small)]" data-test="radio-window">
            <span class="font-mono text-primary">ch {{ w.channel }}</span>
            <span class="text-secondary">{{ w.frames }} frames</span>
            <span v-if="w.kernel_dropped" class="text-state-running">{{ w.kernel_dropped }} missed</span>
            <span class="text-muted">{{ w.frames === 0 ? w.stop.replace(/_/g, " ") : w.channel_matched ? RADIO_WINDOW_ON_CHANNEL : RADIO_WINDOW_OFF_CHANNEL }}</span>
          </li>
        </ul>
      </div>
    </div>
  </Panel>
</template>
