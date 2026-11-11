<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import Panel from "../common/layout/Panel.vue"
import TrafficMap from "./TrafficMap.vue"
import TrafficRouteSteps from "./TrafficRouteSteps.vue"
import { useRouteTrace } from "../../composables/netaudit/useRouteTrace"
import { motionScroll } from "../../utils/motion"
import { routePoints } from "../../utils/netaudit/traffic"
import {
  ROUTE_MAP_TITLE, ROUTE_REPEAT_BUTTON, ROUTE_REPEAT_EXPLAIN, ROUTE_RUNNING, ROUTE_UNSUPPORTED, TRACE_CARD_ADDRESS_ONLY, TRACE_CARD_BUTTON, TRACE_CARD_INTRO, TRACE_CARD_LABEL,
  TRACE_CARD_PICK, TRACE_CARD_PLACEHOLDER, TRACE_CARD_PRESETS, TRACE_CARD_PRESETS_LABEL, TRACE_CARD_TITLE, TRAFFIC_MAP_ROUTE_NOTE,
} from "../../constants/netaudit"

// "Trace a route": the routers between this computer and any internet address the operator types or picks (Constitution I.14), with the route on a
// map. It needs no traffic look. An address, never a name: a name would need a lookup of its own. The server checks the address, the guardrails
// and the blocked list, and asks for the passphrase; what it refuses it says in words, shown here as it is.
const { prefill, cardAddress, supported, starting, running, error, traceOf, start } = useRouteTrace()
const typed = ref("")
// The card shows the route it started, not every kept trace: one opened from a place is shown with that place.
const trace = computed(() => (cardAddress.value ? traceOf(cardAddress.value) : null))
const points = computed(() => routePoints(trace.value))
const busy = computed(() => starting.value || running.value)
const address = computed(() => typed.value.trim())

async function go(repeat: boolean) {
  if (!address.value || busy.value) return
  if (await start(address.value, repeat)) cardAddress.value = address.value
}
const pick = (a: string) => { typed.value = a }

// Another part of the page (the router's table) can ask for an address to be filled in: take it, bring the card into view, leave the press to the operator.
const cardEl = ref<HTMLElement | null>(null)
watch(prefill, async (a) => {
  if (!a) return
  typed.value = a
  prefill.value = null
  await nextTick()
  cardEl.value?.scrollIntoView?.({ behavior: motionScroll("smooth"), block: "start" })
}, { immediate: true })
</script>

<template>
  <Panel :title="TRACE_CARD_TITLE">
    <div ref="cardEl" class="flex scroll-mt-4 flex-col gap-3" data-test="trace-card">
      <p class="m-0 text-secondary">{{ TRACE_CARD_INTRO }}</p>
      <p v-if="!supported" class="m-0 text-[length:var(--text-small)] text-muted" data-test="trace-unsupported">{{ ROUTE_UNSUPPORTED }}</p>
      <form v-else class="flex flex-col gap-2" @submit.prevent="go(false)">
        <FormField :label="TRACE_CARD_LABEL" :hint="TRACE_CARD_ADDRESS_ONLY">
          <template #default="{ id, describedBy }">
            <input
              :id="id"
              v-model="typed"
              type="text"
              inputmode="decimal"
              autocomplete="off"
              autocapitalize="none"
              spellcheck="false"
              :placeholder="TRACE_CARD_PLACEHOLDER"
              :aria-describedby="describedBy"
              class="h-11 w-full max-w-sm min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8"
              data-test="trace-address"
            />
          </template>
        </FormField>
        <div class="flex flex-wrap items-center gap-2" role="group" :aria-label="TRACE_CARD_PRESETS_LABEL">
          <span class="text-[length:var(--text-small)] text-muted">{{ TRACE_CARD_PRESETS_LABEL }}</span>
          <button
            v-for="p in TRACE_CARD_PRESETS"
            :key="p.address"
            type="button"
            class="inline-flex min-h-11 items-center gap-2 rounded-full border border-hairline px-3 text-[length:var(--text-small)] text-secondary hover:bg-surface-hover sm:min-h-8"
            :aria-label="TRACE_CARD_PICK(p.label, p.address)"
            data-test="trace-preset"
            @click="pick(p.address)"
          >{{ p.label }} <span class="font-mono text-muted">{{ p.address }}</span></button>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <BaseButton type="submit" variant="secondary" size="sm" class-name="max-sm:!h-11" :loading="busy" :disabled="busy || !address" data-test="trace-go">{{ busy ? ROUTE_RUNNING : TRACE_CARD_BUTTON }}</BaseButton>
          <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy || !address" data-test="trace-repeat" @click="go(true)">{{ ROUTE_REPEAT_BUTTON }}</BaseButton>
        </div>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ ROUTE_REPEAT_EXPLAIN }}</p>
      </form>
      <p v-if="error" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="trace-error">{{ error }}</p>
      <div v-if="trace" class="flex flex-col gap-3 border-t border-hairline pt-3" data-test="trace-result">
        <p class="m-0 break-all font-mono text-primary" data-test="trace-result-address">{{ trace.address }}</p>
        <TrafficRouteSteps :trace="trace" />
        <div v-if="points.length" class="flex flex-col gap-1" data-test="trace-map">
          <h3 class="m-0 font-mono text-[length:var(--text-micro)] font-normal uppercase tracking-[var(--tracking-micro)] text-muted">{{ ROUTE_MAP_TITLE }}</h3>
          <TrafficMap :dots="[]" :route="points" />
          <p v-if="points.length > 1" class="m-0 text-[length:var(--text-small)] text-muted">{{ TRAFFIC_MAP_ROUTE_NOTE }}</p>
        </div>
      </div>
    </div>
  </Panel>
</template>
