<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Callout from "../common/feedback/Callout.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import StatusTag from "../common/display/StatusTag.vue"
import Icon from "../icons/Icon.vue"
import NearbyDeviceDetail from "./NearbyDeviceDetail.vue"
import { useNearby } from "../../composables/netaudit/useNearby"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { copyText } from "../../utils/clipboard"
import { toNetwork } from "../../router/routes"
import {
  NEARBY_ADVERTISES_AS, NEARBY_CHECK_NOW, NEARBY_CHECKING, NEARBY_CLOSE_DETAIL, NEARBY_COPIED, NEARBY_COPY, NEARBY_COPY_FAILED, NEARBY_COPY_HINT, NEARBY_FILTER_LABEL,
  NEARBY_FILTER_LABELS, NEARBY_FILTERS, NEARBY_GROUP_TITLES, NEARBY_KIND_ICONS, NEARBY_NO_MATCH, NEARBY_OFF_BODY, NEARBY_READ_FAILED_TITLE, NEARBY_TITLE, NEARBY_TURN_ON,
} from "../../constants/netaudit"
import { filterNearby, groupNearby, nearbyCounts, nearbyKind, nearbyKindLabel, nearbyName, nearbyNote, nearbyReport, nearbyStatus, nearbySummary, serviceLabel } from "../../utils/netaudit/nearby"
import { formatRelativeTime } from "../../utils/format/time"
import type { NearbyDevice, NearbyFilter, NearbyStatus } from "../../types/netaudit"

// Network · Devices · Nearby: what the devices on the network say they are. Simple is one line (how many were heard and how many
// need a look), a problem pinned above the list, and a row per device; Advanced is a table of the raw facts with a button that
// copies them. A row opens its details beside the list on a wide screen and in a sheet from the bottom on a phone. The switch is
// on the Settings tab. Everything a device said is shown as text only.
const { state, error, busy, load, check } = useNearby()
const { advanced } = useNetauditViewMode()
const filter = ref<NearbyFilter>("all")
const selected = ref<string | null>(null)
const copied = ref("")

const devices = computed(() => (state.value ? filterNearby(state.value, filter.value) : []))
const groups = computed(() => groupNearby(devices.value, state.value?.last_look))
const counts = computed(() => (state.value ? nearbyCounts(state.value) : null))
const filterOptions = computed(() => NEARBY_FILTERS.map((value) => ({ value, label: `${NEARBY_FILTER_LABELS[value]}${counts.value ? ` (${counts.value[value]})` : ""}` })))
const pinned = computed(() => state.value?.devices.find((d) => d.problem))
const current = computed(() => state.value?.devices.find((d) => d.ip === selected.value) ?? null)
const busyLooking = computed(() => busy.value || !!state.value?.running)
const TAG: Record<NearbyStatus, "error" | "info" | "success" | "warning"> = { attention: "error", new: "info", heard: "success", quiet: "warning" }
const MAX_CHIPS = 3

const open = (d: NearbyDevice) => (selected.value = selected.value === d.ip ? null : d.ip)
const statusOf = (d: NearbyDevice) => nearbyStatus(d, state.value?.last_look)
const shown = (d: NearbyDevice) => (d.services ?? []).slice(0, MAX_CHIPS)
const copy = async () => {
  copied.value = (await copyText(JSON.stringify(state.value ? nearbyReport(state.value) : {}, null, 2))) ? NEARBY_COPIED : NEARBY_COPY_FAILED
}
onMounted(load)
</script>

<template>
  <Panel :title="NEARBY_TITLE">
    <div class="flex flex-col gap-3" data-test="nearby">
      <ErrorState v-if="error && !state" :title="NEARBY_READ_FAILED_TITLE" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!state" label="Reading what devices said" />

      <template v-else-if="!state.enabled">
        <p class="m-0 text-secondary" data-test="nearby-off">{{ NEARBY_OFF_BODY }}</p>
        <RouterLink :to="toNetwork('settings')" class="inline-flex min-h-11 items-center self-start text-accent-info-text underline-offset-2 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring sm:min-h-8" data-test="nearby-turn-on">{{ NEARBY_TURN_ON }}</RouterLink>
      </template>

      <template v-else>
        <Callout v-if="pinned?.problem" tone="error" :title="pinned.problem.title" data-test="nearby-pinned">
          {{ pinned.problem.text }}
          <template #actions><BaseButton variant="secondary" class-name="max-sm:!h-11" data-test="nearby-pinned-fix" @click="selected = pinned.ip">Show the device</BaseButton></template>
        </Callout>

        <div class="flex flex-wrap items-start justify-between gap-2">
          <div class="flex min-w-0 flex-1 flex-col gap-0.5">
            <p class="m-0 text-secondary" data-test="nearby-summary">{{ nearbySummary(state) }}</p>
            <p v-if="nearbyNote(state)" class="m-0 text-[length:var(--text-small)] text-muted">{{ nearbyNote(state) }}</p>
          </div>
          <BaseButton variant="primary" icon="refresh" class-name="max-sm:!h-11 max-sm:w-full" :disabled="busyLooking" data-test="nearby-check" @click="check">{{ busyLooking ? NEARBY_CHECKING : NEARBY_CHECK_NOW }}</BaseButton>
        </div>
        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>

        <template v-if="state.devices.length">
          <div>
            <SegmentedControl v-model="filter" :options="filterOptions" :label="NEARBY_FILTER_LABEL" class="max-sm:hidden" />
            <label class="flex flex-col gap-1 sm:hidden">
              <span class="text-[length:var(--text-small)] text-muted">{{ NEARBY_FILTER_LABEL }}</span>
              <select v-model="filter" class="h-11 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 text-primary focus-visible:outline-none focus-visible:ring-2" data-test="nearby-filter">
                <option v-for="o in filterOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
              </select>
            </label>
          </div>

          <p v-if="!devices.length" class="m-0 text-secondary" data-test="nearby-no-match">{{ NEARBY_NO_MATCH }}</p>

          <div v-else-if="advanced" class="flex flex-col gap-2">
            <div class="overflow-x-auto" data-test="nearby-table">
              <table class="w-full border-collapse text-[length:var(--text-small)]">
                <thead>
                  <tr class="text-left text-muted">
                    <th class="px-2 py-1.5 font-medium">Name</th><th class="px-2 py-1.5 font-medium">Name from</th><th class="px-2 py-1.5 font-medium">Service types</th>
                    <th class="px-2 py-1.5 font-medium">UPnP device type</th><th class="px-2 py-1.5 font-medium">Ports</th><th class="px-2 py-1.5 font-medium">Address</th><th class="px-2 py-1.5 font-medium">Last heard</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="d in devices" :key="d.ip" class="border-t border-hairline align-top">
                    <td class="break-words px-2 py-1.5 text-primary">{{ nearbyName(d) }}</td>
                    <td class="px-2 py-1.5">{{ (d.sources ?? []).join(", ") || "—" }}</td>
                    <td class="break-all px-2 py-1.5 font-mono">{{ (d.services ?? []).join(", ") || "—" }}</td>
                    <td class="break-all px-2 py-1.5 font-mono">{{ d.upnp_type || "—" }}</td>
                    <td class="px-2 py-1.5 font-mono">{{ (d.ports ?? []).join(", ") || "—" }}</td>
                    <td class="px-2 py-1.5 font-mono">{{ d.ip }}</td>
                    <td class="whitespace-nowrap px-2 py-1.5">{{ formatRelativeTime(d.last_heard) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <BaseButton variant="ghost" size="sm" icon="copy" class-name="max-sm:!h-11" data-test="nearby-copy" @click="copy">{{ NEARBY_COPY }}</BaseButton>
              <span role="status" class="text-[length:var(--text-small)] text-muted">{{ copied || NEARBY_COPY_HINT }}</span>
            </div>
          </div>

          <div v-else class="grid min-w-0 gap-3 lg:grid-cols-[minmax(0,1fr)_18rem]" data-test="nearby-list">
            <div class="flex min-w-0 flex-col gap-3">
              <section v-for="g in groups" :key="g.status" :aria-label="NEARBY_GROUP_TITLES[g.status]" class="flex flex-col gap-1">
                <h3 class="m-0 font-mono text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)] text-muted">{{ NEARBY_GROUP_TITLES[g.status] }} · {{ g.devices.length }}</h3>
                <ul class="m-0 flex list-none flex-col border border-hairline p-0">
                  <li v-for="d in g.devices" :key="d.ip" class="border-t border-hairline first:border-t-0">
                    <button type="button" class="flex min-h-14 w-full min-w-0 items-start gap-3 bg-transparent px-3 py-2 text-left text-inherit hover:bg-surface-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring" :class="selected === d.ip ? 'bg-surface-hover' : ''" :aria-pressed="selected === d.ip" data-test="nearby-row" @click="open(d)">
                      <span class="mt-0.5 flex h-8 w-8 flex-none items-center justify-center border border-strong text-secondary" :title="`${NEARBY_ADVERTISES_AS} ${nearbyKindLabel(d)}`"><Icon :name="NEARBY_KIND_ICONS[nearbyKind(d)]" size="sm" /></span>
                      <span class="flex min-w-0 flex-1 flex-col gap-1">
                        <span class="break-words font-semibold text-primary" data-test="nearby-name">{{ nearbyName(d) }}</span>
                        <span class="text-[length:var(--text-small)] text-muted">{{ NEARBY_ADVERTISES_AS }} {{ nearbyKindLabel(d) }} · {{ d.ip }}</span>
                        <span class="flex flex-wrap items-center gap-1.5">
                          <StatusTag :state="TAG[statusOf(d)]" :label="NEARBY_GROUP_TITLES[statusOf(d)]" />
                          <span v-for="s in shown(d)" :key="s" class="bg-surface-hover px-1.5 text-[length:var(--text-micro)] text-secondary">{{ serviceLabel(s) }}</span>
                          <span v-if="(d.services?.length ?? 0) > MAX_CHIPS" class="text-[length:var(--text-micro)] text-muted">+{{ (d.services?.length ?? 0) - MAX_CHIPS }}</span>
                        </span>
                        <span v-if="d.problem" class="text-[length:var(--text-small)] text-state-error">{{ d.problem.text }}</span>
                      </span>
                    </button>
                  </li>
                </ul>
              </section>
            </div>
            <div v-if="current" class="hidden h-fit border border-hairline p-3 lg:block" data-test="nearby-panel">
              <NearbyDeviceDetail :device="current" :advanced="advanced" />
            </div>
          </div>
        </template>
      </template>
    </div>

    <div v-if="current && !advanced" class="fixed inset-0 z-40 flex items-end bg-scrim/50 lg:hidden" data-test="nearby-sheet" @click.self="selected = null">
      <div role="dialog" aria-modal="true" :aria-label="nearbyName(current)" class="flex max-h-[85vh] w-full flex-col gap-3 overflow-y-auto border-t border-strong bg-surface p-4">
        <NearbyDeviceDetail :device="current" :advanced="advanced" />
        <BaseButton variant="secondary" class-name="max-sm:!h-11" data-test="nearby-close" @click="selected = null">{{ NEARBY_CLOSE_DETAIL }}</BaseButton>
      </div>
    </div>
  </Panel>
</template>
