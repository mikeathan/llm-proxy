<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import { useDeviceLookups } from "../../composables/netaudit/useDeviceLookups"
import { useDeviceNames } from "../../composables/netaudit/useDeviceNames"
import { deviceLabel } from "../../utils/netaudit/devices"
import { flagWords, groupLookups } from "../../utils/netaudit/lookups"
import { formatRelativeTime } from "../../utils/format/time"
import { toNetworkSettings } from "../../router/routes"
import {
  LOOKUPS_CLOSE, LOOKUPS_COUNT, LOOKUPS_EMPTY, LOOKUPS_FEWER, LOOKUPS_FLAG_HELP, LOOKUPS_INTRO, LOOKUPS_LAST, LOOKUPS_NO_NAMES, LOOKUPS_OFF, LOOKUPS_OPEN, LOOKUPS_OPEN_SETTINGS,
  LOOKUPS_REFRESH, LOOKUPS_SHOW_ALL, LOOKUPS_UNKNOWN_DEVICE,
} from "../../constants/netaudit"
import type { DeviceLookups } from "../../types/netaudit"

// What each device on this network asked the router's DNS for, from the Phone-home watch (Constitution I.8). It is lookups, not traffic: the router
// line has no address and no byte count, and nothing here is looked up again from this computer. A device opens to its names grouped by domain,
// newest first. Names are text a device chose, so they are shown as text only and never sent anywhere.
const { devices, enabled, loaded, loading, error, load } = useDeviceLookups()
const { deviceByMac, load: loadDevices } = useDeviceNames()
onMounted(() => { void load(); void loadDevices() })

const GROUPS_SHOWN = 8
const open = ref<string | null>(null)
const showAll = ref(false)
const toggle = (mac: string) => { open.value = open.value === mac ? null : mac; showAll.value = false }
const refresh = () => { void load(); void loadDevices() }

const labelOf = (d: DeviceLookups) => {
  const known = deviceByMac(d.mac)
  return known ? deviceLabel(known) : LOOKUPS_UNKNOWN_DEVICE
}
const addressOf = (d: DeviceLookups) => deviceByMac(d.mac)?.ip ?? d.mac
const groups = computed(() => {
  const d = devices.value.find((x) => x.mac === open.value)
  return d ? groupLookups(d.names) : []
})
const shownGroups = computed(() => (showAll.value ? groups.value : groups.value.slice(0, GROUPS_SHOWN)))
</script>

<template>
  <div class="flex flex-col gap-3" data-test="lookups">
    <p class="m-0 text-secondary">{{ LOOKUPS_INTRO }}</p>
    <div>
      <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :loading="loading" :disabled="loading" data-test="lookups-refresh" @click="refresh">{{ LOOKUPS_REFRESH }}</BaseButton>
    </div>
      <p v-if="error" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="lookups-error">{{ error }}</p>
      <div v-if="loaded && !enabled" class="flex flex-wrap items-center gap-3" data-test="lookups-off">
        <p class="m-0 text-secondary">{{ LOOKUPS_OFF }}</p>
        <RouterLink :to="toNetworkSettings('outside')" class="inline-flex min-h-11 items-center text-accent-info-text underline-offset-2 hover:underline sm:min-h-8">{{ LOOKUPS_OPEN_SETTINGS }}</RouterLink>
      </div>
      <EmptyState v-else-if="loaded && !devices.length && !error" title="Nothing asked yet" :body="LOOKUPS_EMPTY" data-test="lookups-empty" />
      <ul v-else-if="devices.length" class="m-0 flex list-none flex-col p-0" data-test="lookups-devices">
        <li v-for="d in devices" :key="d.mac" class="border-t border-hairline" data-test="lookups-device">
          <button
            type="button"
            class="flex w-full min-w-0 flex-col gap-1 bg-transparent py-2 text-left max-sm:min-h-11"
            :aria-expanded="open === d.mac"
            :aria-label="open === d.mac ? LOOKUPS_CLOSE(labelOf(d)) : LOOKUPS_OPEN(labelOf(d))"
            data-test="lookups-toggle"
            @click="toggle(d.mac)"
          >
            <span class="flex flex-wrap items-baseline justify-between gap-x-3">
              <span class="min-w-0 break-words text-primary">{{ labelOf(d) }} <span class="font-mono text-[length:var(--text-small)] text-muted">{{ addressOf(d) }}</span></span>
              <span class="text-[length:var(--text-small)] text-muted">{{ LOOKUPS_COUNT(d.today, d.total) }}<template v-if="d.last"> · {{ LOOKUPS_LAST(formatRelativeTime(d.last)) }}</template></span>
            </span>
            <span v-if="d.flags.length" class="flex flex-wrap gap-1.5" data-test="lookups-flags">
              <span v-for="f in flagWords(d.flags)" :key="f.flag" :title="LOOKUPS_FLAG_HELP[f.flag]" class="rounded-full border border-hairline px-2 text-[length:var(--text-small)] text-state-running">{{ f.label }}</span>
            </span>
          </button>
          <div v-if="open === d.mac" class="flex flex-col gap-2 pb-3 pl-3" data-test="lookups-names">
            <p v-if="!d.names.length" class="m-0 text-[length:var(--text-small)] text-muted">{{ LOOKUPS_NO_NAMES }}</p>
            <ul v-else class="m-0 flex list-none flex-col gap-2 p-0">
              <li v-for="g in shownGroups" :key="g.domain" class="flex flex-col" data-test="lookups-group">
                <span class="flex flex-wrap items-baseline gap-x-2">
                  <span class="break-all font-mono text-primary">{{ g.domain }}</span>
                  <span v-if="g.flag" class="text-[length:var(--text-small)] text-state-running">{{ flagWords([g.flag])[0]!.label }}</span>
                  <span class="text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(g.last) }}</span>
                </span>
                <span v-if="g.names.length > 1 || g.names[0]!.name !== g.domain" class="break-all text-[length:var(--text-small)] text-muted">{{ g.names.map((n) => n.name).join(", ") }}</span>
              </li>
            </ul>
            <div v-if="groups.length > GROUPS_SHOWN">
              <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="lookups-more" @click="showAll = !showAll">{{ showAll ? LOOKUPS_FEWER : LOOKUPS_SHOW_ALL(groups.length - GROUPS_SHOWN) }}</BaseButton>
            </div>
          </div>
        </li>
      </ul>
  </div>
</template>
