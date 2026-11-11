<script setup lang="ts">
import { computed } from "vue"
import { bytesOf, destinationLabel, kindsByAddress, ownerLine, placeGroups, splitPercent } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import {
  TRAFFIC_GROUP_SHARE, TRAFFIC_NO_NAME, TRAFFIC_OPEN_PLACE, TRAFFIC_OTHERS, TRAFFIC_PLACE_GROUPS, TRAFFIC_PLACE_KINDS, TRAFFIC_KIND_SHORT,
} from "../../constants/netaudit"
import type { HostTraffic, TrafficDestination } from "../../types/netaudit"

// The places the data went, in three lists so a person reads them as one story each: the internet (with who runs each server and in which country),
// devices on this network (with what the person calls them) and the shared addresses many devices listen to. Every row says what kinds of traffic
// it had and opens the detail of the place. Names are text a peer chose, so they are shown as text only.
const props = defineProps<{ traffic: HostTraffic; deviceNames: Readonly<Record<string, string>>; openAddress: string | null }>()
const emit = defineEmits<{ (e: "open", address: string): void }>()

const groups = computed(() => placeGroups(props.traffic.destinations))
const kinds = computed(() => kindsByAddress(props.traffic.flows))
const total = computed(() => props.traffic.bytes_in + props.traffic.bytes_out + props.traffic.bytes_through)
const lists = computed(() => (["internet", "network", "shared"] as const)
  .map((id) => ({ id, ...TRAFFIC_PLACE_GROUPS[id], places: groups.value[id], bytes: bytesOf(groups.value[id]), biggest: Math.max(1, ...groups.value[id].map((d) => d.bytes)) }))
  .filter((l) => l.places.length > 0))
const share = (bytes: number) => splitPercent(bytes, Math.max(0, total.value - bytes))[0]
const barWidth = (bytes: number, biggest: number) => `${Math.max(2, Math.round((bytes / biggest) * 100))}%`
// The line under a place's name: who runs an internet server and where, or what the person calls a device on this network.
const detailLine = (d: TrafficDestination): string => (d.internet ? ownerLine(d.owner) : (props.deviceNames[d.address] ?? ""))
const kindLine = (d: TrafficDestination): string => TRAFFIC_PLACE_KINDS((kinds.value.get(d.address) ?? []).map((k) => TRAFFIC_KIND_SHORT[k]))
</script>

<template>
  <div class="flex flex-col gap-5" data-test="places">
    <section v-for="l in lists" :key="l.id" :aria-label="l.title" :data-test="`places-${l.id}`">
      <div class="mb-2 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
        <h3 class="m-0 font-mono text-[length:var(--text-micro)] font-normal uppercase tracking-[var(--tracking-micro)] text-muted">{{ l.title }}</h3>
        <span class="text-[length:var(--text-small)] text-muted" data-test="places-share">{{ TRAFFIC_GROUP_SHARE(share(l.bytes), formatBytes(l.bytes)) }}</span>
      </div>
      <ul class="m-0 flex list-none flex-col gap-3 p-0" data-test="destinations">
        <li v-for="d in l.places" :key="d.address + (d.name ?? '')" class="flex flex-col gap-1" data-test="destination">
          <button type="button" class="flex w-full flex-col gap-1 bg-transparent p-0 text-left max-sm:min-h-11" :aria-label="TRAFFIC_OPEN_PLACE(destinationLabel(d).text)" :aria-expanded="openAddress === d.address" data-test="destination-open" @click="emit('open', d.address)">
            <span class="flex w-full items-baseline justify-between gap-3">
              <span class="min-w-0 break-all text-primary"><span data-test="destination-name">{{ destinationLabel(d).text }}</span><span v-if="!destinationLabel(d).named && d.internet" class="ml-1 text-muted">({{ TRAFFIC_NO_NAME }})</span></span>
              <span class="flex-none text-[length:var(--text-small)] text-muted">{{ formatBytes(d.bytes) }}</span>
            </span>
            <span v-if="detailLine(d) || kindLine(d)" class="flex flex-wrap gap-x-2 text-[length:var(--text-small)] text-muted">
              <span v-if="detailLine(d)" class="min-w-0 break-words" :data-test="d.internet ? 'destination-owner' : 'destination-device'">{{ detailLine(d) }}</span>
              <span v-if="kindLine(d)" data-test="destination-kinds">{{ kindLine(d) }}</span>
            </span>
            <span class="block h-2 bg-surface-hover" aria-hidden="true"><span class="block h-2 bg-accent-info" :style="{ width: barWidth(d.bytes, l.biggest) }"></span></span>
          </button>
        </li>
      </ul>
    </section>
    <p v-if="traffic.other_flows" class="m-0 text-[length:var(--text-small)] text-muted" data-test="destinations-other">{{ TRAFFIC_OTHERS(traffic.other_flows) }} · {{ formatBytes(traffic.other_bytes) }}</p>
  </div>
</template>
