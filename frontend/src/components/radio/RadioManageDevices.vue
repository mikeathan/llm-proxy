<script setup lang="ts">
import { onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import Callout from "../common/feedback/Callout.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import StackBoardLink from "../stacks/StackBoardLink.vue"
import { useRadioDevices } from "../../composables/radio/useRadioDevices"
import { useConfirm } from "../../composables/ui/useConfirm"
import type { RadioClass, RadioOwnerDevice } from "../../types/radio"
import {
  RADIO_ADD_CLOSE, RADIO_ADD_HINT, RADIO_ADD_OPEN, RADIO_CLASS_ORDER, RADIO_CLASS_PICK, RADIO_CLASS_WORDS, RADIO_DEVICE_ADD, RADIO_DEVICE_ADDRESS, RADIO_DEVICE_LABEL,
  RADIO_DEVICE_REMOVE, RADIO_MANAGE_EMPTY, RADIO_MANAGE_TITLE, RADIO_REMOVE_TITLE, radioRemoveBody,
} from "../../constants/radio"

// Radio · Add or remove a device: the control side of My devices. The devices marked as yours, each with Remove, and adding one by its address
// (folded away: Find my device above is the usual way). How they are doing (home or away, which router box) is the dashboard's My devices card.
const d = useRadioDevices()
const { confirm } = useConfirm()
const adding = ref(false)
const address = ref("")
const label = ref("")
const cls = ref("")
const classOptions = [{ value: "", label: "Let the rules decide" }, ...RADIO_CLASS_ORDER.filter((c) => c !== "unknown").map((c) => ({ value: c, label: RADIO_CLASS_WORDS[c] }))]
const INPUT = "h-8 w-full min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 font-mono text-[length:var(--text-small)] text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2 max-sm:h-11"

async function add(): Promise<void> {
  await d.add(address.value, label.value, (cls.value || undefined) as RadioClass | undefined)
  if (!d.error.value) { address.value = ""; label.value = ""; cls.value = ""; adding.value = false }
}

async function remove(device: RadioOwnerDevice): Promise<void> {
  if (!(await confirm({ title: RADIO_REMOVE_TITLE, message: radioRemoveBody(device.label), type: "warning", confirmText: RADIO_DEVICE_REMOVE }))) return
  await d.remove(device.id)
}

onMounted(() => { void d.load() })
</script>

<template>
  <Panel :title="RADIO_MANAGE_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-manage">
      <StackBoardLink stack="radio-devices" />
      <Callout v-if="d.error.value" tone="error" title="That did not work"><span data-test="radio-devices-error">{{ d.error.value }}</span></Callout>
      <ul v-if="d.devices.value?.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0" data-test="radio-manage-list">
        <li v-for="x in d.devices.value" :key="x.id" class="flex items-center justify-between gap-3 py-2" data-test="radio-manage-device">
          <span class="min-w-0 break-words text-primary" data-test="radio-manage-name">{{ x.label }}</span>
          <BaseButton variant="secondary" class-name="flex-none max-sm:!h-11" :disabled="d.busy.value" data-test="radio-my-device-remove" @click="remove(x)">{{ RADIO_DEVICE_REMOVE }}</BaseButton>
        </li>
      </ul>
      <p v-else-if="d.devices.value" class="m-0 text-secondary" data-test="radio-devices-empty">{{ RADIO_MANAGE_EMPTY }}</p>
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
