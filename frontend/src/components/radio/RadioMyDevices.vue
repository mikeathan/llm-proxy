<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import Callout from "../common/feedback/Callout.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import RadioMyDeviceRow from "./RadioMyDeviceRow.vue"
import { useRadioDevices } from "../../composables/radio/useRadioDevices"
import { useRadioPresence } from "../../composables/radio/useRadioPresence"
import { useRadioLive } from "../../composables/radio/useRadioLive"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { useConfirm } from "../../composables/ui/useConfirm"
import { duplicateNames, myDeviceRows } from "../../utils/radio/myDevices"
import type { MyDeviceRow } from "../../types/radio"
import { toRadio } from "../../router/routes"
import type { RadioClass } from "../../types/radio"
import {
  RADIO_ADD_CLOSE, RADIO_ADD_HINT, RADIO_ADD_OPEN, RADIO_CLASS_ORDER, RADIO_CLASS_PICK, RADIO_CLASS_WORDS, RADIO_DEVICES_EMPTY, RADIO_DEVICES_INTRO,
  RADIO_DEVICES_TITLE, RADIO_DEVICE_ADD, RADIO_DEVICE_ADDRESS, RADIO_DEVICE_LABEL, RADIO_DEVICE_REMOVE, RADIO_DUP_TITLE, RADIO_MY_START, RADIO_REMOVE_TITLE,
  radioDupLine, radioMySummary, radioRemoveBody,
} from "../../constants/radio"

// Radio · My devices: your devices, home ones first, each a row that opens its page. Three reads make the rows: your devices, where they were
// hour by hour, and the live picture (the router box and "moving now"; a failed live read only leaves those out). Two names alike are shown as
// a problem with its fix. Adding by address is folded away: Find my device on Around you is the usual way.
const d = useRadioDevices()
const p = useRadioPresence()
const live = useRadioLive()
const { advanced } = useNetauditViewMode()
const { confirm } = useConfirm()
const adding = ref(false)
const address = ref("")
const label = ref("")
const cls = ref("")
const classOptions = [{ value: "", label: "Let the rules decide" }, ...RADIO_CLASS_ORDER.filter((c) => c !== "unknown").map((c) => ({ value: c, label: RADIO_CLASS_WORDS[c] }))]
const INPUT = "h-8 w-full min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 font-mono text-[length:var(--text-small)] text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2 max-sm:h-11"

const rows = computed(() => myDeviceRows(d.devices.value ?? [], p.devices.value, live.view.value?.devices ?? []))
const listening = computed(() => !!live.view.value?.listening)
const here = computed(() => rows.value.filter((r) => r.state === "here").length)
const dups = computed(() => duplicateNames(rows.value))

async function add(): Promise<void> {
  await d.add(address.value, label.value, (cls.value || undefined) as RadioClass | undefined)
  if (!d.error.value) { address.value = ""; label.value = ""; cls.value = ""; adding.value = false }
}

async function remove(r: MyDeviceRow): Promise<void> {
  if (!(await confirm({ title: RADIO_REMOVE_TITLE, message: radioRemoveBody(r.label), type: "warning", confirmText: RADIO_DEVICE_REMOVE }))) return
  await d.remove(r.id)
  void p.load()
}

onMounted(() => { void d.load(); void p.load(); void live.load() })
</script>

<template>
  <Panel :title="RADIO_DEVICES_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-devices">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_DEVICES_INTRO }}</p>
      <Callout v-if="d.error.value" tone="error" title="That did not work"><span data-test="radio-devices-error">{{ d.error.value }}</span></Callout>
      <template v-if="rows.length">
        <p class="m-0 flex flex-wrap items-center gap-x-3 gap-y-1 text-primary" data-test="radio-my-summary">
          <span>{{ radioMySummary(here, rows.length, listening) }}</span>
          <RouterLink v-if="live.view.value && !listening" :to="toRadio()" class="text-[length:var(--text-small)] text-accent-brand hover:underline" data-test="radio-my-start">{{ RADIO_MY_START }}</RouterLink>
        </p>
        <Callout v-if="dups.length" tone="warning" :title="RADIO_DUP_TITLE">
          <ul class="m-0 flex list-none flex-col gap-1 p-0" data-test="radio-my-dups">
            <li v-for="x in dups" :key="x.label">{{ radioDupLine(x.label, x.count) }}</li>
          </ul>
        </Callout>
        <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" data-test="radio-my-list">
          <RadioMyDeviceRow v-for="r in rows" :key="r.id" :row="r" :advanced="advanced" :busy="d.busy.value" @remove="remove" />
        </ul>
      </template>
      <p v-else-if="d.devices.value" class="m-0 text-secondary" data-test="radio-devices-empty">{{ RADIO_DEVICES_EMPTY }}</p>
      <BaseButton v-if="!adding" variant="secondary" class-name="self-start max-sm:!h-11" data-test="radio-add-open" @click="adding = true">{{ RADIO_ADD_OPEN }}</BaseButton>
      <form v-else class="flex flex-col gap-3 rounded-[var(--radius-md)] bg-surface-raised px-3 py-3" data-test="radio-add-form" @submit.prevent="add">
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_ADD_HINT }}</p>
        <div class="flex flex-wrap items-end gap-3">
          <FormField :label="RADIO_DEVICE_LABEL"><template #default="{ id }"><input :id="id" v-model="label" type="text" maxlength="60" :class="INPUT" data-test="radio-device-label" /></template></FormField>
          <FormField :label="RADIO_DEVICE_ADDRESS"><template #default="{ id }"><input :id="id" v-model="address" type="text" autocomplete="off" spellcheck="false" :class="INPUT" data-test="radio-device-address" /></template></FormField>
          <FormField :label="RADIO_CLASS_PICK"><template #default><SelectInput v-model="cls" :options="classOptions" :label="RADIO_CLASS_PICK" /></template></FormField>
        </div>
        <div class="flex flex-wrap gap-2">
          <BaseButton variant="primary" type="submit" class-name="max-sm:!h-11" :disabled="d.busy.value || !address || !label" data-test="radio-device-add">{{ RADIO_DEVICE_ADD }}</BaseButton>
          <BaseButton variant="secondary" class-name="max-sm:!h-11" data-test="radio-add-close" @click="adding = false">{{ RADIO_ADD_CLOSE }}</BaseButton>
        </div>
      </form>
    </div>
  </Panel>
</template>
