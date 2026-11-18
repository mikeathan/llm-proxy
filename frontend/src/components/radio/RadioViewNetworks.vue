<script setup lang="ts">
import { onMounted } from "vue"
import RadioMemberRow from "./RadioMemberRow.vue"
import { useRadioAroundNetworks } from "../../composables/radio/useRadioAroundNetworks"
import { usePolling } from "../../composables/ui/usePolling"
import { radioDbmLine } from "../../constants/radio"
import {
  RADIO_NETWORK_HIDDEN, RADIO_NETWORKS_LOADING, RADIO_NETWORKS_NONE, RADIO_NETWORKS_OTHER, RADIO_NETWORKS_UNCONNECTED, RADIO_NETWORKS_UNCONNECTED_HELP,
  RADIO_NETWORKS_YOURS, radioClientsLine, radioRadiosLine,
} from "../../constants/radioIdentity"
import { brandModel, nameOf, shortBrand } from "../../utils/radio/identity"
import type { RadioNetworkGroup } from "../../types/radio"

// Radio · view "Networks": every router around the house as a card (one card per box: its 2.4 GHz, 5 GHz and guest radios together), with the
// devices that use it underneath. Yours first. This is how a device is told apart: what it is connected to. Every line opens the device's page.
const nets = useRadioAroundNetworks()
const REFRESH_MS = 30000

function title(g: RadioNetworkGroup): string {
  const r = g.radios[0]
  if (!r) return RADIO_NETWORK_HIDDEN
  return brandModel(r.ident) || (r.ident?.brand ? `${shortBrand(r.ident.brand)} router` : nameOf(r))
}

onMounted(() => { void nets.load() })
usePolling(() => { void nets.load() }, REFRESH_MS)
</script>

<template>
  <div class="flex flex-col gap-3" data-test="radio-view-networks">
    <p v-if="nets.error.value" class="m-0 text-state-error" data-test="radio-networks-error">{{ nets.error.value }}</p>
    <p v-else-if="!nets.view.value" class="m-0 text-muted">{{ RADIO_NETWORKS_LOADING }}</p>
    <template v-else>
      <p v-if="!nets.view.value.groups.length" class="m-0 text-secondary" data-test="radio-networks-none">{{ RADIO_NETWORKS_NONE }}</p>
      <section
        v-for="g in nets.view.value.groups" :key="g.id" class="flex flex-col gap-1 rounded-[var(--radius-md)] border px-3 py-2"
        :class="g.yours ? 'border-accent-brand' : 'border-hairline'" data-test="radio-network-group" :data-yours="g.yours ? 1 : 0"
      >
        <header class="flex flex-wrap items-baseline justify-between gap-x-3">
          <span class="flex min-w-0 flex-wrap items-baseline gap-x-2">
            <span class="text-primary" data-test="radio-network-title">{{ title(g) }}</span>
            <span class="text-[length:var(--text-small)]" :class="g.yours ? 'text-accent-brand' : 'text-muted'">{{ g.yours ? RADIO_NETWORKS_YOURS : RADIO_NETWORKS_OTHER }}</span>
          </span>
          <span class="font-mono text-[length:var(--text-small)] text-muted">{{ radioDbmLine(g.strongest_dbm) }}</span>
        </header>
        <p class="m-0 text-[length:var(--text-small)] text-secondary" data-test="radio-network-names">{{ g.names.length ? g.names.join(" · ") : RADIO_NETWORK_HIDDEN }} — {{ radioRadiosLine(g.radios.length) }}, {{ radioClientsLine(g.clients.length) }}</p>
        <details v-if="g.radios.length > 1" data-test="radio-network-radios">
          <summary class="min-h-11 cursor-pointer list-none text-[length:var(--text-small)] text-accent-brand focus-visible:outline-none focus-visible:ring-2 sm:min-h-8">{{ radioRadiosLine(g.radios.length) }}</summary>
          <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0"><li v-for="m in g.radios" :key="m.id"><RadioMemberRow :member="m" /></li></ul>
        </details>
        <ul v-if="g.clients.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0" data-test="radio-network-clients">
          <li v-for="m in g.clients" :key="m.id"><RadioMemberRow :member="m" /></li>
        </ul>
        <RadioMemberRow v-if="g.radios.length === 1 && g.radios[0]" :member="g.radios[0]" />
      </section>
      <details v-if="nets.view.value.unconnected.length" class="rounded-[var(--radius-md)] border border-hairline px-3 py-2" data-test="radio-networks-unconnected-box">
        <summary class="min-h-11 cursor-pointer list-none text-secondary focus-visible:outline-none focus-visible:ring-2 sm:min-h-8">{{ RADIO_NETWORKS_UNCONNECTED }} ({{ nets.view.value.unconnected.length }})</summary>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_NETWORKS_UNCONNECTED_HELP }}</p>
        <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" data-test="radio-networks-unconnected"><li v-for="m in nets.view.value.unconnected" :key="m.id"><RadioMemberRow :member="m" /></li></ul>
      </details>
    </template>
  </div>
</template>
