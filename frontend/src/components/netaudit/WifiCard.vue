<script setup lang="ts">
import { onMounted, reactive, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import ModuleSwitchNote from "../stacks/ModuleSwitchNote.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useWifi } from "../../composables/netaudit/useWifi"
import {
  WIFI_ADD, WIFI_CONFIRM, WIFI_DETAILS, WIFI_HINT, WIFI_LOOK, WIFI_NAME_LABEL, WIFI_NONE_SEEN, WIFI_NOT_LOOKED, WIFI_REMOVE, WIFI_STEPS, WIFI_TITLE,
  WIFI_UNCONFIRMED_NOTE, wifiSeenLine,
} from "../../constants/netaudit"
import { formatRelativeTime, isRecorded } from "../../utils/format/time"

// Settings · Network · Wi-Fi look-alikes: the operator's own network names, a look at what is advertising them, and the choice of which
// access points are theirs. The switch is on the Modules tab. Confirming and stopping a watch ask for the passphrase (the server says so).
const { state, error, busy, load, look, addNetwork, removeNetwork, confirm } = useWifi()
const name = ref("")
const setupOpen = ref(false)
// The addresses ticked for each name, until they are confirmed.
const ticked = reactive<Record<string, string[]>>({})

async function add(): Promise<void> {
  if (await addNetwork(name.value.trim())) name.value = ""
}
function toggle(network: string, bssid: string, on: boolean): void {
  const rest = (ticked[network] ?? []).filter((a) => a !== bssid)
  ticked[network] = on ? [...rest, bssid] : rest
}
async function confirmTicked(network: string): Promise<void> {
  if (await confirm(network, ticked[network] ?? [])) ticked[network] = []
}
onMounted(load)
</script>

<template>
  <Panel :title="WIFI_TITLE">
    <div class="flex flex-col gap-3" data-test="wifi">
      <ErrorState v-if="error && !state" title="The Wi-Fi look-alikes setting could not be read" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!state" label="Reading the Wi-Fi look-alikes setting" />

      <template v-else>
        <ModuleSwitchNote :enabled="state.enabled" :hint="WIFI_HINT" :details="WIFI_DETAILS" />
        <p v-if="!state.ready" class="m-0 text-[length:var(--text-small)] text-state-running" data-test="wifi-why">{{ state.why }}</p>
        <p v-if="state.problem" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="wifi-problem">The last look failed: {{ state.problem }}</p>

        <form class="flex flex-wrap items-end gap-2" data-test="wifi-add" @submit.prevent="add">
          <label class="flex min-w-0 flex-1 flex-col gap-1 text-[length:var(--text-small)] text-secondary">
            {{ WIFI_NAME_LABEL }}
            <input v-model="name" type="text" maxlength="32" autocomplete="off" class="h-11 min-w-0 rounded-[var(--radius-md)] border border-strong bg-surface px-3 text-primary sm:h-9" data-test="wifi-name" />
          </label>
          <BaseButton type="submit" variant="secondary" class-name="max-sm:!h-11" :disabled="busy || !name.trim()">{{ WIFI_ADD }}</BaseButton>
        </form>

        <div v-if="state.networks.length" class="flex flex-wrap items-center gap-3">
          <BaseButton variant="primary" icon="refresh" class-name="max-sm:!h-11" :disabled="busy || !state.ready" data-test="wifi-look" @click="look">{{ WIFI_LOOK }}</BaseButton>
          <span class="text-[length:var(--text-small)] text-muted" data-test="wifi-last">{{ isRecorded(state.last_look ?? "") ? `Last look ${formatRelativeTime(state.last_look!)}.` : WIFI_NOT_LOOKED }}</span>
        </div>

        <ul class="m-0 flex list-none flex-col gap-3 p-0">
          <li v-for="n in state.networks" :key="n.name" class="flex flex-col gap-2 rounded-[var(--radius-md)] border border-hairline p-3" :data-test="`wifi-network-${n.name}`">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="min-w-0 break-words font-medium text-primary">{{ n.name }}</span>
              <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" data-test="wifi-remove" @click="removeNetwork(n.name)">{{ WIFI_REMOVE }}</BaseButton>
            </div>
            <p v-if="!n.confirmed.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="wifi-unconfirmed">{{ WIFI_UNCONFIRMED_NOTE }}</p>
            <p v-if="!n.seen.length" class="m-0 text-[length:var(--text-small)] text-muted">{{ WIFI_NONE_SEEN }}</p>
            <ul v-else class="m-0 flex list-none flex-col p-0">
              <li v-for="a in n.seen" :key="a.bssid" class="flex min-h-11 flex-wrap items-center gap-x-3 gap-y-1 border-t border-hairline py-1.5 first:border-t-0" data-test="wifi-seen">
                <label class="flex min-h-11 min-w-0 items-center gap-2 sm:min-h-8">
                  <input v-if="!a.confirmed" type="checkbox" class="h-4 w-4 flex-none accent-[rgb(var(--accent-brand))]" :checked="(ticked[n.name] ?? []).includes(a.bssid)" :aria-label="`${a.bssid} is mine`" @change="toggle(n.name, a.bssid, ($event.target as HTMLInputElement).checked)" />
                  <span class="font-mono text-[length:var(--text-small)] text-primary">{{ a.bssid }}</span>
                </label>
                <span class="text-[length:var(--text-small)] text-muted">{{ wifiSeenLine(a.signal, a.security) }}</span>
                <span :class="['text-[length:var(--text-small)]', a.confirmed ? 'text-state-success' : 'text-state-running']">{{ a.confirmed ? "Confirmed as yours" : "Not confirmed" }}</span>
              </li>
            </ul>
            <div v-if="n.seen.some((a) => !a.confirmed)">
              <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" :disabled="busy || !(ticked[n.name] ?? []).length" data-test="wifi-confirm" @click="confirmTicked(n.name)">{{ WIFI_CONFIRM }}</BaseButton>
            </div>
          </li>
        </ul>

        <Disclosure title="How to set it up" summary="Three steps" :open="setupOpen" data-test="wifi-setup" @update:open="setupOpen = $event">
          <ol class="m-0 flex list-decimal flex-col gap-1 pl-5 text-[length:var(--text-small)] text-secondary">
            <li v-for="step in WIFI_STEPS" :key="step">{{ step }}</li>
          </ol>
        </Disclosure>

        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="wifi-error">{{ error }}</p>
      </template>
    </div>
  </Panel>
</template>
