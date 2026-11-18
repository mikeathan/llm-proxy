<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import Callout from "../common/feedback/Callout.vue"
import StatusTag from "../common/display/StatusTag.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import { RADIO_AP_GUEST, RADIO_AP_GUEST_HELP } from "../../constants/radioIdentity"
import { useRadioNetworks } from "../../composables/radio/useRadioNetworks"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { networkGroups } from "../../utils/radio/networks"
import {
  RADIO_AP_CONFIRM, RADIO_AP_CONFIRMED, RADIO_AP_REJECT, RADIO_NETWORKS_INTRO, RADIO_NETWORKS_TITLE, RADIO_NETWORK_ADD, RADIO_NETWORK_ADD_CLOSE,
  RADIO_NETWORK_ADD_OPEN, RADIO_NETWORK_ASK_TITLE, RADIO_NETWORK_NAME, RADIO_NETWORK_NONE, RADIO_NETWORK_REMOVE_NAME, radioApLine, radioNetworkAsk, radioNetworkLine,
} from "../../constants/radio"

// Radio · Your Wi-Fi networks: one line per network name, its access points grouped under it (a mesh's boxes are one network). Access points
// that broadcast your name and are not confirmed are asked about at the top, as the one thing to do here. Advanced lists each confirmed access
// point, with Not mine and the guest switch, and the way to remove a name; adding a name is folded away.
const n = useRadioNetworks()
const { advanced } = useNetauditViewMode()
const name = ref("")
const adding = ref(false)
const groups = computed(() => (n.networks.value ? networkGroups(n.networks.value) : []))
const candidates = computed(() => groups.value.flatMap((g) => g.candidates))
const INPUT = "h-8 w-full min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 text-[length:var(--text-small)] text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2 max-sm:h-11"

async function add(): Promise<void> {
  await n.addName(name.value.trim())
  if (!n.error.value) { name.value = ""; adding.value = false }
}

onMounted(() => { void n.load() })
</script>

<template>
  <Panel :title="RADIO_NETWORKS_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-networks">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_NETWORKS_INTRO }}</p>
      <Callout v-if="n.error.value" tone="error" title="That did not work"><span data-test="radio-networks-error">{{ n.error.value }}</span></Callout>
      <Callout v-if="candidates.length" tone="warning" :title="RADIO_NETWORK_ASK_TITLE">
        <div class="flex flex-col gap-2">
          <span>{{ radioNetworkAsk(candidates.length) }}</span>
          <ul class="m-0 flex list-none flex-col gap-2 p-0">
            <li v-for="ap in candidates" :key="ap.id" class="flex flex-wrap items-center gap-2" data-test="radio-ap" data-confirmed="false">
              <span class="min-w-0 break-words text-primary">{{ radioApLine(ap.name, ap.channel, ap.security, ap.last_seen) }}</span>
              <BaseButton variant="primary" class-name="max-sm:!h-11" :disabled="n.busy.value" data-test="radio-ap-confirm" @click="n.confirm(ap.id)">{{ RADIO_AP_CONFIRM }}</BaseButton>
              <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="n.busy.value" data-test="radio-ap-reject" @click="n.reject(ap.id)">{{ RADIO_AP_REJECT }}</BaseButton>
            </li>
          </ul>
        </div>
      </Callout>
      <ul v-if="groups.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="g in groups" :key="g.name" class="flex flex-col gap-1.5 py-2" data-test="radio-network">
          <span class="flex flex-wrap items-center justify-between gap-2">
            <span class="flex min-w-0 flex-col">
              <span class="break-all font-medium text-primary" data-test="radio-network-name">{{ g.name }}</span>
              <span class="text-[length:var(--text-small)] text-muted" data-test="radio-network-line">{{ radioNetworkLine(g.channels, g.confirmed.length) }}</span>
            </span>
            <button v-if="advanced" type="button" class="text-[length:var(--text-small)] text-muted underline max-sm:min-h-11" :disabled="n.busy.value" :aria-label="`${RADIO_NETWORK_REMOVE_NAME} ${g.name}`" data-test="radio-network-remove" @click="n.removeName(g.name)">{{ RADIO_NETWORK_REMOVE_NAME }}</button>
          </span>
          <ul v-if="advanced && g.confirmed.length" class="m-0 flex list-none flex-col gap-1.5 p-0 pl-3">
            <li v-for="ap in g.confirmed" :key="ap.id" class="flex flex-wrap items-center gap-2" data-test="radio-ap" data-confirmed="true">
              <span class="break-words text-[length:var(--text-small)] text-secondary">{{ radioApLine(ap.name, ap.channel, ap.security, ap.last_seen) }}</span>
              <StatusTag state="success" :label="RADIO_AP_CONFIRMED" />
              <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="n.busy.value" data-test="radio-ap-reject" @click="n.reject(ap.id)">{{ RADIO_AP_REJECT }}</BaseButton>
              <BaseToggle :model-value="!!ap.guest" :label="RADIO_AP_GUEST" :title="RADIO_AP_GUEST_HELP" data-test="radio-ap-guest" @update:model-value="n.setGuest(ap.id, $event)" />
            </li>
          </ul>
        </li>
      </ul>
      <p v-else-if="n.networks.value" class="m-0 text-secondary" data-test="radio-networks-empty">{{ RADIO_NETWORK_NONE }}</p>
      <BaseButton v-if="!adding" variant="secondary" class-name="self-start max-sm:!h-11" data-test="radio-network-add-open" @click="adding = true">{{ RADIO_NETWORK_ADD_OPEN }}</BaseButton>
      <form v-else class="flex flex-wrap items-end gap-3" @submit.prevent="add">
        <FormField :label="RADIO_NETWORK_NAME"><template #default="{ id }"><input :id="id" v-model="name" type="text" maxlength="32" autocomplete="off" :class="INPUT" data-test="radio-network-input" /></template></FormField>
        <BaseButton variant="primary" type="submit" class-name="max-sm:!h-11" :disabled="n.busy.value || !name.trim()" data-test="radio-network-add">{{ RADIO_NETWORK_ADD }}</BaseButton>
        <BaseButton variant="secondary" class-name="max-sm:!h-11" @click="adding = false">{{ RADIO_NETWORK_ADD_CLOSE }}</BaseButton>
      </form>
    </div>
  </Panel>
</template>
