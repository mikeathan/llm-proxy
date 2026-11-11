<script setup lang="ts">
import { computed } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import TrafficRouteSteps from "./TrafficRouteSteps.vue"
import { useRouteTrace } from "../../composables/netaudit/useRouteTrace"
import { ROUTE_AGAIN, ROUTE_BUTTON, ROUTE_EXPLAIN, ROUTE_REPEAT_BUTTON, ROUTE_RUNNING, ROUTE_TITLE, ROUTE_UNSUPPORTED } from "../../constants/netaudit"

// The route from this computer to one internet address (Constitution I.14), from the detail of a place the look saw: traced only when the operator
// presses; the server asks for the passphrase. The steps are TrafficRouteSteps, which the "Trace a route" card shows too.
const props = defineProps<{ address: string }>()
const { supported, starting, running, error, traceOf, start } = useRouteTrace()
const trace = computed(() => traceOf(props.address))
const busy = computed(() => starting.value || running.value)
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
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" data-test="route-repeat" @click="start(address, true)">{{ ROUTE_REPEAT_BUTTON }}</BaseButton>
      </div>
    </template>
    <p v-if="error" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="route-error">{{ error }}</p>
    <TrafficRouteSteps v-if="trace" :trace="trace" />
  </section>
</template>
