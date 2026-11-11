<script setup lang="ts">
import { computed } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useRouteTrace } from "../../composables/netaudit/useRouteTrace"
import { hopWords, routeOutcome } from "../../utils/netaudit/traffic"
import {
  ROUTE_AGAIN, ROUTE_BUTTON, ROUTE_EXPLAIN, ROUTE_HOP_LOSS, ROUTE_RUNNING, ROUTE_SILENT_NOTE, ROUTE_STEPS_LABEL, ROUTE_TITLE, ROUTE_UNSUPPORTED,
} from "../../constants/netaudit"

// The route from this computer to one internet address (Constitution I.14): the routers that answered at each step, who runs each and where it is
// used (from the tables kept on this computer), and how quickly it answered. Traced only when the operator presses; the server asks for the
// passphrase. A step where no router answered is shown as such, never dropped.
const props = defineProps<{ address: string }>()
const { supported, starting, running, error, traceOf, start } = useRouteTrace()
const trace = computed(() => traceOf(props.address))
const busy = computed(() => starting.value || running.value)
const outcome = computed(() => (trace.value ? routeOutcome(trace.value) : ""))
const anySilent = computed(() => trace.value?.hops.some((h) => !h.address) ?? false)
</script>

<template>
  <section class="flex flex-col gap-2" :aria-label="ROUTE_TITLE" data-test="route">
    <h3 class="m-0 font-mono text-[length:var(--text-micro)] font-normal uppercase tracking-[var(--tracking-micro)] text-muted">{{ ROUTE_TITLE }}</h3>
    <p v-if="!supported" class="m-0 text-[length:var(--text-small)] text-muted" data-test="route-unsupported">{{ ROUTE_UNSUPPORTED }}</p>
    <template v-else>
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ ROUTE_EXPLAIN }}</p>
      <div class="flex flex-wrap items-center gap-3">
        <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" :loading="busy" :disabled="busy" data-test="route-start" @click="start(address)">
          {{ trace?.state === "running" ? ROUTE_RUNNING : trace ? ROUTE_AGAIN : ROUTE_BUTTON }}
        </BaseButton>
      </div>
    </template>
    <p v-if="error" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="route-error">{{ error }}</p>
    <ol v-if="trace && trace.hops.length" class="m-0 flex list-none flex-col p-0" :aria-label="ROUTE_STEPS_LABEL" data-test="route-hops">
      <li v-for="h in trace.hops" :key="h.ttl" class="grid grid-cols-[2rem_1fr_auto] items-baseline gap-x-2 border-t border-hairline py-1.5" data-test="route-hop">
        <span class="font-mono text-[length:var(--text-small)] text-muted">{{ h.ttl }}</span>
        <span class="flex min-w-0 flex-col">
          <span class="break-all" :class="h.address ? 'font-mono text-primary' : 'text-muted'" data-test="route-hop-who">{{ hopWords(h).who }}</span>
          <span v-if="hopWords(h).where" class="break-words text-[length:var(--text-small)] text-muted" data-test="route-hop-where">{{ hopWords(h).where }}</span>
        </span>
        <span class="text-right text-[length:var(--text-small)] text-muted">
          <span v-if="hopWords(h).time" class="block" data-test="route-hop-time">{{ hopWords(h).time }}</span>
          <span v-if="h.address && h.answered < h.sent" class="block">{{ ROUTE_HOP_LOSS(h.answered, h.sent) }}</span>
        </span>
      </li>
    </ol>
    <p v-if="outcome" class="m-0 text-secondary" data-test="route-outcome">{{ outcome }}</p>
    <p v-if="anySilent && trace?.state !== 'running'" class="m-0 text-[length:var(--text-small)] text-muted">{{ ROUTE_SILENT_NOTE }}</p>
  </section>
</template>
