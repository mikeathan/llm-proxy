<script setup lang="ts">
import { computed, onMounted } from "vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ModuleTile from "./ModuleTile.vue"
import { useStack } from "../../composables/stacks/useStack"
import { LEFT_BLOCKED_PREFIX, STACK_MODULES_INTRO, moduleElementId, stackSummaryLine } from "../../constants/stacks"
import { layerNumber } from "../../utils/stacks/modules"
import { toNetworkSettings } from "../../router/routes"
import { MODULE_SETTINGS_GROUP } from "../../constants/netaudit"
import { NETWORK_SETTINGS_GROUPS } from "../../types/routes"
import type { StackId } from "../../types/stacks"

// A stack's Modules tab: the one place its modules are switched (ADR-0028). Layers are drawn top to bottom, each module a
// tile (a row on a phone) with its switch and one status line. A switch that the server refuses leaves the module as it was
// and says why beside it.
const props = defineProps<{ stack: StackId }>()
const { stack, error, errorFor, busyModule, leftBlocked, load, set } = useStack(props.stack)

const summary = computed(() => stack.value?.summary)
const titles = computed<Record<string, string>>(() => Object.fromEntries((stack.value?.layers ?? []).flatMap((l) => l.modules.map((m) => [m.id, m.title]))))

// A pointer from a blocked module takes you to the module it needs: scrolled into view and focused, so a keyboard or screen reader follows.
function goTo(id: string): void {
  const el = document.getElementById(moduleElementId(id))
  el?.scrollIntoView({ block: "center", behavior: "smooth" })
  el?.focus({ preventScroll: true })
}
onMounted(load)
</script>

<template>
  <div class="flex flex-col gap-4" data-test="stack-modules">
    <p class="m-0 max-w-prose text-[length:var(--text-small)] text-muted">{{ STACK_MODULES_INTRO }}</p>

    <ErrorState v-if="error && !stack" title="The modules could not be read" :cause="error" next="Check that the server is running, then retry.">
      <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
    </ErrorState>
    <LoadingState v-else-if="!stack" label="Reading the modules" :rows="3" />

    <template v-else>
      <p v-if="summary" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="stack-summary">
        {{ stackSummaryLine(summary.on, summary.total) }}
        <template v-if="summary.stopped"> &middot; <span class="text-state-running">{{ summary.stopped }} stopped</span></template>
        <template v-if="summary.blocked"> &middot; <span class="text-muted">{{ summary.blocked }} blocked</span></template>
      </p>
      <p v-if="leftBlocked.length" role="status" class="m-0 text-[length:var(--text-small)] text-muted" data-test="left-blocked">{{ LEFT_BLOCKED_PREFIX }}{{ leftBlocked.join(", ") }}.</p>

      <section
        v-for="(layer, index) in stack.layers"
        :key="layer.id"
        :aria-labelledby="`layer-${layer.id}`"
        class="flex flex-col gap-2 border-t border-hairline pt-3 md:grid md:grid-cols-[11rem_minmax(0,1fr)] md:gap-5"
        :data-test="`layer-${layer.id}`"
      >
        <div class="flex flex-col gap-0.5">
          <h3 :id="`layer-${layer.id}`" class="m-0 font-mono text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)] text-accent-info-text">
            {{ layerNumber(index, stack.layers.length) }} &mdash; {{ layer.title }}
          </h3>
          <span v-if="layer.purpose" class="text-[length:var(--text-small)] text-faint">{{ layer.purpose }}</span>
        </div>
        <ul class="m-0 flex list-none flex-col divide-y divide-hairline rounded-[var(--radius-md)] bg-surface-raised p-0 md:grid md:grid-cols-2 md:gap-3 md:divide-y-0 md:bg-transparent xl:grid-cols-4">
          <ModuleTile
            v-for="m in layer.modules"
            :key="m.id"
            :module="m"
            :settings-to="props.stack === 'network' ? toNetworkSettings(MODULE_SETTINGS_GROUP[m.id] ?? NETWORK_SETTINGS_GROUPS[0]) : undefined"
            :titles="titles"
            :busy="busyModule === m.id"
            :error="errorFor?.module === m.id ? errorFor.message : undefined"
            @switch="set(m.id, $event)"
            @goto="goTo"
          />
        </ul>
      </section>
    </template>
  </div>
</template>
