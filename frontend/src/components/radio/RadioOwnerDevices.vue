<script setup lang="ts">
import { onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import Callout from "../common/feedback/Callout.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import RadioPresenceStrip from "./RadioPresenceStrip.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { useRadioDevices } from "../../composables/radio/useRadioDevices"
import { useRadioPresence } from "../../composables/radio/useRadioPresence"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { formatRelativeTime } from "../../utils/format/time"
import type { RadioClass } from "../../types/radio"
import {
  RADIO_CLASS_BY_OWNER, RADIO_CLASS_BY_RULES, RADIO_CLASS_ORDER, RADIO_CLASS_PICK, RADIO_CLASS_WORDS,
  RADIO_DEVICES_EMPTY, RADIO_DEVICES_INTRO, RADIO_DEVICES_TITLE, RADIO_DEVICE_ADD, RADIO_DEVICE_ADDRESS, RADIO_DEVICE_LABEL, RADIO_DEVICE_REMOVE,
  RADIO_FORGET, RADIO_FORGET_BODY, RADIO_JOINED_EVIDENCE, RADIO_NEVER_HEARD, RADIO_NOW_WORDS, RADIO_PRESENCE_EMPTY, RADIO_PRESENCE_INTRO, RADIO_SIGNAL_WORDS,
  radioLastHeardLine,
} from "../../constants/radio"

// Radio · My devices: the devices the owner names as theirs. The address goes in once and is never shown again; the list shows the name and the days seen.
const d = useRadioDevices()
const p = useRadioPresence()
const { advanced } = useNetauditViewMode()
const NOW_STATE = { here: "running", away: "neutral", not_listening: "queued" } as const
const presenceOf = (id: string) => p.devices.value.find((x) => x.id === id)
const address = ref("")
const label = ref("")
const cls = ref("")
const classOptions = [{ value: "", label: "Let the rules decide" }, ...RADIO_CLASS_ORDER.filter((c) => c !== "unknown").map((c) => ({ value: c, label: RADIO_CLASS_WORDS[c] }))]
const INPUT = "h-8 w-full min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 font-mono text-[length:var(--text-small)] text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2 max-sm:h-11"

async function add(): Promise<void> {
  await d.add(address.value, label.value, (cls.value || undefined) as RadioClass | undefined)
  if (!d.error.value) { address.value = ""; label.value = ""; cls.value = "" }
}

onMounted(() => { void d.load(); void p.load() })
</script>

<template>
  <Panel :title="RADIO_DEVICES_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-devices">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_DEVICES_INTRO }}</p>
      <Callout v-if="d.error.value" tone="error" title="That did not work"><span data-test="radio-devices-error">{{ d.error.value }}</span></Callout>
      <p v-if="d.devices.value?.length && p.devices.value.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-presence-intro">{{ RADIO_PRESENCE_INTRO }}</p>
      <ul v-if="d.devices.value?.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="x in d.devices.value" :key="x.id" class="flex flex-wrap items-center justify-between gap-2 py-2" data-test="radio-device">
          <span class="flex min-w-0 flex-col gap-1"><span class="break-words text-primary">{{ x.label }}</span><span class="text-[length:var(--text-small)] text-muted">Seen {{ x.visits }} {{ x.visits === 1 ? "day" : "days" }}, last {{ x.last_seen }} · <span data-test="radio-device-class">{{ RADIO_CLASS_WORDS[x.class] }}</span> ({{ x.class_by === "owner" ? RADIO_CLASS_BY_OWNER : RADIO_CLASS_BY_RULES }})</span></span>
          <div v-if="presenceOf(x.id)" class="order-last flex w-full flex-col gap-1" data-test="radio-device-presence">
            <p class="m-0 flex flex-wrap items-center gap-2 text-[length:var(--text-small)]">
              <StatusTag :state="NOW_STATE[presenceOf(x.id)!.state]" :label="RADIO_NOW_WORDS[presenceOf(x.id)!.state]" />
              <span class="text-muted" data-test="radio-device-last-heard">{{ presenceOf(x.id)!.last_heard ? radioLastHeardLine(formatRelativeTime(presenceOf(x.id)!.last_heard!)) : RADIO_NEVER_HEARD }}</span>
              <span v-if="advanced && presenceOf(x.id)!.signal && presenceOf(x.id)!.signal !== 'unknown'" class="text-muted" data-test="radio-device-signal">{{ RADIO_SIGNAL_WORDS[presenceOf(x.id)!.signal!] }} signal</span>
              <span v-if="advanced && presenceOf(x.id)!.joined_own" class="text-state-success" data-test="radio-device-joined">{{ RADIO_JOINED_EVIDENCE }}</span>
            </p>
            <RadioPresenceStrip :device="presenceOf(x.id)!" />
          </div>
          <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="d.busy.value" data-test="radio-device-remove" @click="d.remove(x.id)">{{ RADIO_DEVICE_REMOVE }}</BaseButton>
        </li>
      </ul>
      <p v-else-if="d.devices.value" class="m-0 text-secondary" data-test="radio-devices-empty">{{ RADIO_DEVICES_EMPTY }} {{ RADIO_PRESENCE_EMPTY }}</p>
      <form class="flex flex-wrap items-end gap-3" @submit.prevent="add">
        <FormField :label="RADIO_DEVICE_LABEL"><template #default="{ id }"><input :id="id" v-model="label" type="text" maxlength="60" :class="INPUT" data-test="radio-device-label" /></template></FormField>
        <FormField :label="RADIO_DEVICE_ADDRESS"><template #default="{ id }"><input :id="id" v-model="address" type="text" autocomplete="off" spellcheck="false" :class="INPUT" data-test="radio-device-address" /></template></FormField>
        <FormField :label="RADIO_CLASS_PICK"><template #default><SelectInput v-model="cls" :options="classOptions" :label="RADIO_CLASS_PICK" /></template></FormField>
        <BaseButton variant="primary" type="submit" class-name="max-sm:!h-11" :disabled="d.busy.value || !address || !label" data-test="radio-device-add">{{ RADIO_DEVICE_ADD }}</BaseButton>
      </form>
      <div class="flex flex-col gap-2 border-t border-hairline pt-3">
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_FORGET_BODY }}</p>
        <p v-if="d.notice.value" class="m-0 text-[length:var(--text-small)] text-state-success" data-test="radio-forget-done">{{ d.notice.value }}</p>
        <div><BaseButton variant="danger" class-name="max-sm:!h-11" :disabled="d.busy.value" data-test="radio-forget" @click="d.forget">{{ RADIO_FORGET }}</BaseButton></div>
      </div>
    </div>
  </Panel>
</template>
