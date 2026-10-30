<script setup lang="ts">
import { onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useDecoys } from "../../composables/netaudit/useDecoys"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { DECOYS_FIREWALL, DECOYS_HINT, DECOYS_NONE, DECOYS_SCANNER_NOTE, DECOYS_TITLE } from "../../constants/netaudit"
import { formatRelativeTime } from "../../utils/format/time"

// Settings · Network scan · Decoy services: the switch, which fake services listen, and who connected to one. Simple is the
// switch and the list of who; Advanced adds the ports.
const { state, error, busy, load, set } = useDecoys()
const { advanced } = useNetauditViewMode()

onMounted(load)
</script>

<template>
  <Panel :title="DECOYS_TITLE">
    <div class="flex flex-col gap-3" data-test="decoys">
      <ErrorState v-if="error && !state" title="The decoy services could not be read" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!state" label="Reading the decoy services" />

      <template v-else>
        <ToggleField :model-value="state.enabled" label="Run decoy services" :hint="DECOYS_HINT" :disabled="busy" @update:model-value="set" />

        <p v-if="state.enabled && state.problem" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="decoy-problem">Some decoys cannot start: {{ state.problem }}</p>
        <p v-if="state.enabled" class="m-0 text-[length:var(--text-small)] text-muted">{{ DECOYS_FIREWALL }}</p>

        <ul v-if="advanced" class="m-0 flex list-none flex-col gap-0.5 p-0" aria-label="Decoy ports">
          <li v-for="port in state.ports" :key="port.number" class="flex flex-wrap justify-between gap-x-3 text-[length:var(--text-small)]">
            <span class="text-secondary">{{ port.name }}</span>
            <span class="font-mono text-muted">{{ port.number }}{{ state.listening.includes(port.number) ? " · listening" : "" }}</span>
          </li>
        </ul>

        <p v-if="!state.touches.length" class="m-0 text-secondary">{{ DECOYS_NONE }}</p>
        <ul v-else class="m-0 flex list-none flex-col gap-1 p-0" aria-label="Devices that connected to a decoy" data-test="decoy-touches">
          <li v-for="t in state.touches" :key="`${t.source}-${t.port}`" class="flex flex-wrap justify-between gap-x-3">
            <span class="font-mono text-primary">{{ t.source }}</span>
            <span class="text-[length:var(--text-small)] text-muted">port {{ t.port }} · {{ t.count }} {{ t.count === 1 ? "time" : "times" }} · last {{ formatRelativeTime(t.last) }}</span>
          </li>
        </ul>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ DECOYS_SCANNER_NOTE }}</p>

        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
      </template>
    </div>
  </Panel>
</template>
