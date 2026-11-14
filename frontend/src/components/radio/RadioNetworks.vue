<script setup lang="ts">
import { onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import Callout from "../common/feedback/Callout.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { useRadioNetworks } from "../../composables/radio/useRadioNetworks"
import {
  RADIO_AP_CANDIDATE, RADIO_AP_CONFIRM, RADIO_AP_CONFIRMED, RADIO_AP_REJECT, RADIO_NETWORKS_INTRO, RADIO_NETWORKS_TITLE, RADIO_NETWORK_ADD,
  RADIO_NETWORK_NAME, RADIO_NETWORK_NONE, RADIO_NETWORK_REMOVE_NAME, radioApLine,
} from "../../constants/radio"

// Radio · Your networks: names of the owner's own Wi-Fi. A name only proposes access points; the owner confirms which are theirs.
const n = useRadioNetworks()
const name = ref("")
const INPUT = "h-8 w-full min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 text-[length:var(--text-small)] text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2 max-sm:h-11"

async function add(): Promise<void> {
  await n.addName(name.value.trim())
  if (!n.error.value) name.value = ""
}

onMounted(() => { void n.load() })
</script>

<template>
  <Panel :title="RADIO_NETWORKS_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-networks">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_NETWORKS_INTRO }}</p>
      <Callout v-if="n.error.value" tone="error" title="That did not work"><span data-test="radio-networks-error">{{ n.error.value }}</span></Callout>
      <ul v-if="n.networks.value?.names.length" class="m-0 flex list-none flex-wrap gap-2 p-0">
        <li v-for="x in n.networks.value.names" :key="x" class="flex items-center gap-2 rounded-[var(--radius-md)] border border-hairline px-2.5 py-1" data-test="radio-network-name">
          <span class="break-all text-primary">{{ x }}</span>
          <button type="button" class="text-[length:var(--text-small)] text-muted underline max-sm:min-h-11" :disabled="n.busy.value" :aria-label="`${RADIO_NETWORK_REMOVE_NAME} ${x}`" @click="n.removeName(x)">{{ RADIO_NETWORK_REMOVE_NAME }}</button>
        </li>
      </ul>
      <p v-else-if="n.networks.value" class="m-0 text-secondary" data-test="radio-networks-empty">{{ RADIO_NETWORK_NONE }}</p>
      <form class="flex flex-wrap items-end gap-3" @submit.prevent="add">
        <FormField :label="RADIO_NETWORK_NAME"><template #default="{ id }"><input :id="id" v-model="name" type="text" maxlength="32" autocomplete="off" :class="INPUT" data-test="radio-network-input" /></template></FormField>
        <BaseButton variant="primary" type="submit" class-name="max-sm:!h-11" :disabled="n.busy.value || !name.trim()" data-test="radio-network-add">{{ RADIO_NETWORK_ADD }}</BaseButton>
      </form>
      <ul v-if="n.networks.value?.access_points.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="ap in n.networks.value.access_points" :key="ap.id" class="flex flex-col gap-1.5 py-2" data-test="radio-ap" :data-confirmed="ap.confirmed">
          <span class="break-words text-[length:var(--text-small)] text-primary">{{ radioApLine(ap.name, ap.channel, ap.security, ap.last_seen) }}</span>
          <span v-if="ap.confirmed" class="flex items-center gap-2"><StatusTag state="success" :label="RADIO_AP_CONFIRMED" /><BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="n.busy.value" data-test="radio-ap-reject" @click="n.reject(ap.id)">{{ RADIO_AP_REJECT }}</BaseButton></span>
          <span v-else class="flex flex-wrap items-center gap-2">
            <span class="text-[length:var(--text-small)] text-muted">{{ RADIO_AP_CANDIDATE }}</span>
            <BaseButton variant="primary" class-name="max-sm:!h-11" :disabled="n.busy.value" data-test="radio-ap-confirm" @click="n.confirm(ap.id)">{{ RADIO_AP_CONFIRM }}</BaseButton>
            <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="n.busy.value" data-test="radio-ap-reject" @click="n.reject(ap.id)">{{ RADIO_AP_REJECT }}</BaseButton>
          </span>
        </li>
      </ul>
    </div>
  </Panel>
</template>
