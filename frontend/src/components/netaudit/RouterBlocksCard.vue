<script setup lang="ts">
import { computed } from "vue"
import StatCard from "../common/display/StatCard.vue"
import Sparkline from "../common/display/Sparkline.vue"
import { portsLine } from "../../utils/netaudit/activity"
import { ROUTER_CARD_LABEL, ROUTER_CARD_NO_PORTS, ROUTER_CARD_NOTE, ROUTER_CARD_TITLE } from "../../constants/netaudit"
import type { RouterBlocks } from "../../types/netaudit"

// Connection attempts from the internet that the router's own log says it refused: today's count, the ports most tried and the
// last days. A figure to watch and never an alert: the log is unauthenticated, and the card says what the number is and is not.
const props = defineProps<{ router: RouterBlocks }>()
const trend = computed(() => props.router.days.map((d) => d.total))
const ports = computed(() => portsLine(props.router.today_ports))
</script>

<template>
  <section :aria-label="ROUTER_CARD_TITLE" class="flex flex-col gap-2 border-b border-hairline/60 px-4 py-3" data-test="router-card">
    <div class="grid max-w-3xl grid-cols-[repeat(auto-fit,minmax(min(100%,220px),1fr))] gap-px border border-hairline bg-border-hairline">
      <StatCard :label="ROUTER_CARD_LABEL" :value="router.today.toLocaleString('en-US')" :caption="ports || ROUTER_CARD_NO_PORTS">
        <Sparkline :values="trend" label="Attempts the router refused" unit="a day" state="info" />
      </StatCard>
    </div>
    <details class="max-w-3xl text-[length:var(--text-small)] text-muted">
      <summary class="flex min-h-11 cursor-pointer items-center sm:min-h-0">What is this number?</summary>
      <p class="m-0 pb-1">{{ ROUTER_CARD_NOTE }}</p>
    </details>
  </section>
</template>
