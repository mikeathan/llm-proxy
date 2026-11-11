<script setup lang="ts">
import { computed } from "vue"
import { kindShares } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import { TRAFFIC_KINDS_LABEL } from "../../constants/netaudit"
import type { HostTraffic } from "../../types/netaudit"

// What the traffic was, by kind, as bars: each kind's share of everything counted. It is the short answer; the conversations behind it are under
// Advanced. A bar is decoration for the numbers beside it, which carry the meaning.
const props = defineProps<{ traffic: HostTraffic }>()
const shares = computed(() => kindShares(props.traffic.kinds))
</script>

<template>
  <ul class="m-0 flex list-none flex-col gap-2 p-0" :aria-label="TRAFFIC_KINDS_LABEL" data-test="kinds-bars">
    <li v-for="k in shares" :key="k.kind" class="flex flex-col gap-1" data-test="kinds-bar">
      <div class="flex flex-wrap items-baseline justify-between gap-x-3">
        <span class="text-secondary">{{ k.label }}</span>
        <span class="text-[length:var(--text-small)] text-muted">{{ k.percent < 1 ? "under 1" : k.percent }}% · {{ formatBytes(k.bytes) }}</span>
      </div>
      <div class="h-1.5 w-full overflow-hidden rounded-full bg-surface-hover" aria-hidden="true">
        <div class="h-full rounded-full bg-accent-info" :style="{ width: `${Math.max(k.percent, 1)}%` }" />
      </div>
    </li>
  </ul>
</template>
