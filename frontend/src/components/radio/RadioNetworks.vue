<script setup lang="ts">
import { computed, onMounted } from "vue"
import { RouterLink } from "vue-router"
import Panel from "../common/layout/Panel.vue"
import Callout from "../common/feedback/Callout.vue"
import StatusTag from "../common/display/StatusTag.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import { RADIO_AP_GUEST, RADIO_AP_GUEST_HELP } from "../../constants/radioIdentity"
import { useRadioNetworks } from "../../composables/radio/useRadioNetworks"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { networkGroups } from "../../utils/radio/networks"
import { toNetworkSettings } from "../../router/routes"
import {
  RADIO_AP_CONFIRMED, RADIO_AP_CONFIRMED_BEFORE, RADIO_NETWORKS_INTRO, RADIO_NETWORKS_OPEN, RADIO_NETWORKS_TITLE, RADIO_NETWORK_ASK_TITLE,
  RADIO_NETWORK_NONE, radioApLine, radioNetworkAsk, radioNetworkLine,
} from "../../constants/radio"

// Radio · Your Wi-Fi networks: one line per network name, its access points grouped under it (a mesh's boxes are one network). The names and
// which access points are yours are the Network page's one list (Wi-Fi look-alikes); this view reads it and links there, so there is one place
// to confirm. Access points heard under your names and not confirmed are counted at the top. Advanced lists each confirmed access point with
// the guest switch, which is Radio's own rule for what counts as evidence.
const n = useRadioNetworks()
const { advanced } = useNetauditViewMode()
const groups = computed(() => (n.networks.value ? networkGroups(n.networks.value) : []))
const candidates = computed(() => groups.value.flatMap((g) => g.candidates))
const wifiSettings = toNetworkSettings("devices")

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
          <ul class="m-0 flex list-none flex-col gap-1 p-0">
            <li v-for="ap in candidates" :key="ap.id" class="flex flex-col" data-test="radio-ap" data-confirmed="false">
              <span class="min-w-0 break-words text-primary">{{ radioApLine(ap.name, ap.channel, ap.security, ap.last_seen) }}</span>
              <span v-if="ap.confirmed_before" class="text-[length:var(--text-small)] text-muted" data-test="radio-ap-before">{{ RADIO_AP_CONFIRMED_BEFORE }}</span>
            </li>
          </ul>
        </div>
      </Callout>
      <ul v-if="groups.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="g in groups" :key="g.name" class="flex flex-col gap-1.5 py-2" data-test="radio-network">
          <span class="flex min-w-0 flex-col">
            <span class="break-all font-medium text-primary" data-test="radio-network-name">{{ g.name }}</span>
            <span class="text-[length:var(--text-small)] text-muted" data-test="radio-network-line">{{ radioNetworkLine(g.channels, g.confirmed.length) }}</span>
          </span>
          <ul v-if="advanced && g.confirmed.length" class="m-0 flex list-none flex-col gap-1.5 p-0 pl-3">
            <li v-for="ap in g.confirmed" :key="ap.id" class="flex flex-wrap items-center gap-2" data-test="radio-ap" data-confirmed="true">
              <span class="break-words text-[length:var(--text-small)] text-secondary">{{ radioApLine(ap.name, ap.channel, ap.security, ap.last_seen) }}</span>
              <StatusTag state="success" :label="RADIO_AP_CONFIRMED" />
              <BaseToggle :model-value="!!ap.guest" :label="RADIO_AP_GUEST" :title="RADIO_AP_GUEST_HELP" data-test="radio-ap-guest" @update:model-value="n.setGuest(ap.id, $event)" />
            </li>
          </ul>
        </li>
      </ul>
      <p v-else-if="n.networks.value" class="m-0 text-secondary" data-test="radio-networks-empty">{{ RADIO_NETWORK_NONE }}</p>
      <RouterLink :to="wifiSettings" class="self-start text-[length:var(--text-small)] text-accent-brand hover:underline max-sm:min-h-11" data-test="radio-networks-open">{{ RADIO_NETWORKS_OPEN }}</RouterLink>
    </div>
  </Panel>
</template>
