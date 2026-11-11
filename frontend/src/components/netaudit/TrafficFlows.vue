<script setup lang="ts">
import { computed, ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { flowsOfKind, kindShares, ownerLine, serviceOf, visibleFlows } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import {
  TRAFFIC_COLUMNS, TRAFFIC_FLOWS_FEWER, TRAFFIC_FLOWS_KINDS_LABEL, TRAFFIC_FLOWS_MORE, TRAFFIC_FLOWS_SHOWN, TRAFFIC_KIND_FILTER, TRAFFIC_KIND_FILTER_CLEAR,
  TRAFFIC_KIND_FILTER_NONE, TRAFFIC_KIND_LABELS, TRAFFIC_KIND_PICK, TRAFFIC_OPEN_PLACE,
} from "../../constants/netaudit"
import type { HostTraffic, TrafficFlow, TrafficKindWord } from "../../types/netaudit"

// What the traffic was: the kinds as filters (each with its share) and, under them, the conversations of the kind picked. A conversation opens the
// detail of its place. The first few are shown; the rest are one press away, so the page is not a wall of rows.
const props = defineProps<{ traffic: HostTraffic }>()
const emit = defineEmits<{ (e: "open", address: string): void }>()

const shares = computed(() => kindShares(props.traffic.kinds))
const kindFilter = ref<TrafficKindWord | null>(null)
const showAll = ref(false)
// What is filtered or expanded belongs to one look: a new look starts clean.
watch(() => props.traffic, () => { kindFilter.value = null; showAll.value = false })
const matching = computed(() => flowsOfKind(props.traffic.flows, kindFilter.value))
const rows = computed(() => visibleFlows(matching.value, showAll.value, TRAFFIC_FLOWS_SHOWN))
const toggleKind = (kind: TrafficKindWord) => { kindFilter.value = kindFilter.value === kind ? null : kind; showAll.value = false }
const flowName = (f: TrafficFlow) => f.name || f.address
// A named kind ("HTTPS") leaves the port out of its words, so it is added; a served or unnamed one already has it.
const portNote = (f: TrafficFlow) => (f.port && !f.served && !serviceOf(f).includes(String(f.port)) ? String(f.port) : "")
</script>

<template>
  <div class="flex flex-col gap-3">
    <ul v-if="shares.length" class="m-0 flex list-none flex-wrap gap-2 p-0" :aria-label="TRAFFIC_FLOWS_KINDS_LABEL" data-test="kinds">
      <li v-for="k in shares" :key="k.kind" data-test="kind">
        <button
          type="button"
          class="inline-flex min-h-11 items-center gap-2 rounded-full border px-3 text-[length:var(--text-small)] sm:min-h-8"
          :class="kindFilter === k.kind ? 'border-accent-brand bg-surface-hover font-medium text-primary' : 'border-hairline text-secondary hover:bg-surface-hover'"
          :aria-pressed="kindFilter === k.kind"
          :aria-label="TRAFFIC_KIND_PICK(TRAFFIC_KIND_LABELS[k.kind])"
          data-test="kind-pick"
          @click="toggleKind(k.kind)"
        >
          <span>{{ TRAFFIC_KIND_LABELS[k.kind] }}</span><span class="text-muted">{{ k.percent }}% · {{ formatBytes(k.bytes) }}</span>
        </button>
      </li>
    </ul>

    <p v-if="kindFilter" class="m-0 flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-secondary" data-test="kind-filter">
      {{ TRAFFIC_KIND_FILTER(TRAFFIC_KIND_LABELS[kindFilter]) }}
      <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="kind-filter-clear" @click="kindFilter = null">{{ TRAFFIC_KIND_FILTER_CLEAR }}</BaseButton>
    </p>
    <p v-if="traffic.flows.length && !rows.shown.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="flows-none">{{ TRAFFIC_KIND_FILTER_NONE }}</p>
    <div v-else-if="rows.shown.length" class="-mx-4 overflow-x-auto">
      <table class="w-full min-w-[30rem] border-collapse text-left" data-test="flows">
        <thead>
          <tr class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">
            <th scope="col" class="px-4 py-2 font-normal">{{ TRAFFIC_COLUMNS.destination }}</th>
            <th scope="col" class="px-2 py-2 font-normal">{{ TRAFFIC_COLUMNS.service }}</th>
            <th scope="col" class="px-2 py-2 text-right font-normal">{{ TRAFFIC_COLUMNS.sent }}</th>
            <th scope="col" class="px-4 py-2 text-right font-normal">{{ TRAFFIC_COLUMNS.received }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="f in rows.shown" :key="`${f.address}|${f.port}|${f.proto}|${f.served ? 's' : 'c'}`" class="cursor-pointer border-t border-hairline hover:bg-surface-hover" data-test="flow" @click="emit('open', f.address)">
            <td class="max-w-[22rem] break-all px-4 py-2 text-primary"><button type="button" class="break-all bg-transparent p-0 text-left text-primary max-sm:min-h-11" :aria-label="TRAFFIC_OPEN_PLACE(flowName(f))" data-test="flow-open">{{ flowName(f) }}</button><span v-if="ownerLine(f.owner)" class="block text-[length:var(--text-small)] text-muted" data-test="flow-owner">{{ ownerLine(f.owner) }}</span></td>
            <td class="px-2 py-2 text-muted" data-test="flow-service">{{ serviceOf(f) }}<span v-if="portNote(f)" class="ml-1 font-mono text-faint">{{ portNote(f) }}</span></td>
            <td class="px-2 py-2 text-right text-secondary">{{ formatBytes(f.bytes_out) }}</td>
            <td class="px-4 py-2 text-right text-secondary">{{ formatBytes(f.bytes_in) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="rows.hidden > 0 || showAll && matching.length > TRAFFIC_FLOWS_SHOWN">
      <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="flows-toggle" @click="showAll = !showAll">{{ showAll ? TRAFFIC_FLOWS_FEWER : TRAFFIC_FLOWS_MORE(rows.hidden) }}</BaseButton>
    </div>
  </div>
</template>
