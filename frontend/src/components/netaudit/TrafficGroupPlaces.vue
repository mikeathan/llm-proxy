<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import TrafficRoute from "./TrafficRoute.vue"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { formatBytes } from "../../utils/format/formatters"
import { locationOf, ownerName } from "../../utils/netaudit/traffic"
import {
  PLACE_NEW, PLACE_NO_LOCATION, PLACE_ROUTE, PLACE_ROUTE_CLOSE, PLACE_ROUTE_FROM_HERE, PLACE_ROUTE_LABEL, PLACE_SHOW_FEWER, PLACE_SHOW_MORE, ROUTER_CONN_COUNT,
} from "../../constants/netaudit"
import type { PlaceGroup } from "../../types/netaudit"

// The places of one company (or country): each address with its service, how much it carried and where it is, and a Route button that opens the
// route to it right under the row. A route is always traced from this computer, never from the device the places belong to, and the page says so.
const SIMPLE_SHOWN = 4
const ADVANCED_SHOWN = 8
const props = defineProps<{ group: PlaceGroup; routeAddress: string | null; fromThisComputer: boolean }>()
const emit = defineEmits<{ (e: "route", address: string): void }>()
const { advanced } = useNetauditViewMode()
const all = ref(false)
const limit = computed(() => (advanced.value ? ADVANCED_SHOWN : SIMPLE_SHOWN))
const shown = computed(() => (all.value ? props.group.rows : props.group.rows.slice(0, limit.value)))
const hidden = computed(() => props.group.rows.length - shown.value.length)
const where = (r: PlaceGroup["rows"][number]): string => (r.owner ? locationOf(r.owner) : "") || PLACE_NO_LOCATION
const holder = (r: PlaceGroup["rows"][number]): string => (r.owner ? `AS${r.owner.asn} · ${ownerName(r.owner)}` : "")
const size = (r: PlaceGroup["rows"][number]): string => (r.bytes !== undefined ? formatBytes(r.bytes) : ROUTER_CONN_COUNT(r.count))
</script>

<template>
  <ul class="m-0 flex list-none flex-col p-0" data-test="group-places">
    <li v-for="r in shown" :key="`${r.address}|${r.port ?? ''}|${r.proto ?? ''}`" class="flex flex-col gap-2 border-t border-hairline py-2 first:border-t-0" data-test="group-place">
      <div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <span class="min-w-0 break-all font-mono text-primary" data-test="place-address">{{ r.address }}<span v-if="r.port" class="text-[length:var(--text-small)] text-muted">:{{ r.port }} {{ r.proto }}</span></span>
        <span v-if="r.fresh" class="rounded-full border border-state-running px-1.5 text-[length:var(--text-micro)] text-state-running" data-test="place-new">{{ PLACE_NEW }}</span>
        <span class="flex items-baseline gap-3 text-[length:var(--text-small)] text-muted">
          <span>{{ size(r) }}<template v-if="r.state && r.count === 1"> · {{ r.state }}</template></span>
          <button
            type="button"
            class="min-h-11 bg-transparent px-1 text-accent-info-text underline-offset-2 hover:underline sm:min-h-8"
            :aria-expanded="routeAddress === r.address"
            :aria-label="PLACE_ROUTE_LABEL(r.address)"
            data-test="place-route"
            @click="emit('route', r.address)"
          >{{ routeAddress === r.address ? PLACE_ROUTE_CLOSE : PLACE_ROUTE }}</button>
        </span>
      </div>
      <span class="break-words text-[length:var(--text-small)] text-muted" data-test="place-where">{{ r.name ? `${r.name} · ` : "" }}{{ where(r) }}<template v-if="advanced && holder(r)"> · {{ holder(r) }}</template></span>
      <div v-if="routeAddress === r.address" class="flex flex-col gap-2 rounded-[var(--radius-md)] bg-surface-hover p-3" data-test="place-route-panel">
        <p v-if="fromThisComputer" class="m-0 text-[length:var(--text-small)] text-muted" data-test="place-route-from">{{ PLACE_ROUTE_FROM_HERE }}</p>
        <TrafficRoute :address="r.address" />
      </div>
    </li>
    <li v-if="group.rows.length > limit">
      <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="places-more" @click="all = !all">{{ all ? PLACE_SHOW_FEWER : PLACE_SHOW_MORE(hidden) }}</BaseButton>
    </li>
  </ul>
</template>
