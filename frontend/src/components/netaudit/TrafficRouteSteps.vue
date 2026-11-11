<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { foldRoute, hopWords, routeOutcome, routeSummary, statsWords } from "../../utils/netaudit/traffic"
import { ROUTE_HOP_LOSS, ROUTE_REPEAT_RUNNING, ROUTE_SILENT_NOTE, ROUTE_SILENT_RUN, ROUTE_STATS_COLUMNS, ROUTE_STEPS_FOLDED, ROUTE_STEPS_LABEL, ROUTE_STEPS_SHOWN } from "../../constants/netaudit"
import type { RouteHop, RouteTrace } from "../../types/netaudit"

// The steps of one route trace: a line that sums it up, each router that answered with who runs it, where it is used and how quickly it answered
// (and, when the rounds were measured, how steady it was), and the outcome. Runs of steps where no router answered are folded into one line, with
// a switch to see every step: many routers are set never to answer, so the silence is the usual case and not a finding.
const props = defineProps<{ trace: RouteTrace }>()
const showAll = ref(false)
const lines = computed(() => (showAll.value ? props.trace.hops.map((hop) => ({ kind: "hop" as const, hop })) : foldRoute(props.trace.hops)))
const canFold = computed(() => foldRoute(props.trace.hops).some((l) => l.kind === "silent"))
const outcome = computed(() => routeOutcome(props.trace))
const running = computed(() => props.trace.state === "running")
const anySilent = computed(() => props.trace.hops.some((h) => !h.address))
const measuring = computed(() => running.value && props.trace.repeat && (props.trace.rounds_done ?? 0) < (props.trace.rounds ?? 0) && props.trace.hops.some((h) => h.stats))
const stats = (h: RouteHop) => statsWords(h)
</script>

<template>
  <div class="flex flex-col gap-2" data-test="route-steps">
    <p v-if="trace.hops.length && !running" class="m-0 text-secondary" data-test="route-summary">{{ routeSummary(trace) }}</p>
    <p v-if="trace.repeat && running && (trace.rounds_done ?? 0) < (trace.rounds ?? 0) && trace.hops.length" class="m-0 text-[length:var(--text-small)] text-state-running" role="status" data-test="route-rounds">{{ ROUTE_REPEAT_RUNNING((trace.rounds_done ?? 0) + (measuring ? 1 : 0), trace.rounds ?? 0) }}</p>
    <ol v-if="trace.hops.length" class="m-0 flex list-none flex-col p-0" :aria-label="ROUTE_STEPS_LABEL" data-test="route-hops">
      <template v-for="l in lines" :key="l.kind === 'hop' ? l.hop.ttl : `s${l.from}`">
        <li v-if="l.kind === 'hop'" class="grid grid-cols-[2rem_1fr_auto] items-baseline gap-x-2 border-t border-hairline py-1.5" data-test="route-hop">
          <span class="font-mono text-[length:var(--text-small)] text-muted">{{ l.hop.ttl }}</span>
          <span class="flex min-w-0 flex-col">
            <span class="break-all" :class="l.hop.address ? 'font-mono text-primary' : 'text-muted'" data-test="route-hop-who">{{ hopWords(l.hop).who }}</span>
            <span v-if="hopWords(l.hop).where" class="break-words text-[length:var(--text-small)] text-muted" data-test="route-hop-where">{{ hopWords(l.hop).where }}</span>
            <span v-if="stats(l.hop)" class="text-[length:var(--text-small)] text-muted" data-test="route-hop-stats">
              {{ ROUTE_STATS_COLUMNS.answered }} {{ stats(l.hop)!.answered }} · {{ ROUTE_STATS_COLUMNS.best }} {{ stats(l.hop)!.best }} · {{ ROUTE_STATS_COLUMNS.average }} {{ stats(l.hop)!.average }} · {{ ROUTE_STATS_COLUMNS.worst }} {{ stats(l.hop)!.worst }}
            </span>
          </span>
          <span class="text-right text-[length:var(--text-small)] text-muted">
            <span v-if="hopWords(l.hop).time" class="block" data-test="route-hop-time">{{ hopWords(l.hop).time }}</span>
            <span v-if="l.hop.address && l.hop.answered < l.hop.sent" class="block">{{ ROUTE_HOP_LOSS(l.hop.answered, l.hop.sent) }}</span>
          </span>
        </li>
        <li v-else class="grid grid-cols-[2rem_1fr] items-baseline gap-x-2 border-t border-hairline py-1.5" data-test="route-silent-run">
          <span class="font-mono text-[length:var(--text-small)] text-muted">{{ l.from }}–{{ l.to }}</span>
          <span class="text-muted">{{ ROUTE_SILENT_RUN(l.count) }}</span>
        </li>
      </template>
    </ol>
    <div v-if="canFold && !running">
      <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="route-fold-toggle" @click="showAll = !showAll">{{ showAll ? ROUTE_STEPS_FOLDED : ROUTE_STEPS_SHOWN }}</BaseButton>
    </div>
    <p v-if="outcome" class="m-0 text-secondary" data-test="route-outcome">{{ outcome }}</p>
    <p v-if="anySilent && !running" class="m-0 text-[length:var(--text-small)] text-muted">{{ ROUTE_SILENT_NOTE }}</p>
  </div>
</template>
