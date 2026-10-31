<script setup lang="ts">
import { onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import StatusTag from "../common/display/StatusTag.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useDecoys } from "../../composables/netaudit/useDecoys"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { DECOYS_CLEAR_LABEL, DECOYS_CLEAR_MESSAGE, DECOYS_CLEAR_TITLE, DECOYS_CONNECTIONS_TITLE, DECOYS_FIREWALL, DECOYS_HINT, DECOYS_NEEDS_LOOK, DECOYS_SCANNER_NOTE, DECOYS_TITLE } from "../../constants/netaudit"
import { touchSummary } from "../../utils/netaudit/decoys"
import { formatRelativeTime } from "../../utils/format/time"

// Settings · Network scan · Decoy services: the switch, which fake services listen, and who connected to one. Simple is the
// switch and the list of who; Advanced adds the ports.
const { state, error, busy, load, set, clear } = useDecoys()
const { confirm } = useConfirm()
const { advanced } = useNetauditViewMode()

// Who connected is folded to one line, but the moment someone has it opens itself and carries a tag: a connection to a decoy is
// the one thing here that needs a look. The choice is made once, when the state first arrives, so the person's own fold stays.
const open = ref<boolean | null>(null)
watch(state, (s) => { if (open.value === null && s) open.value = s.touches.length > 0 }, { immediate: true })

async function clearList() {
  const ok = await confirm({ title: DECOYS_CLEAR_TITLE, message: DECOYS_CLEAR_MESSAGE, type: "warning", confirmText: DECOYS_CLEAR_LABEL })
  if (ok) await clear()
}

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

        <Disclosure :title="DECOYS_CONNECTIONS_TITLE" :summary="touchSummary(state.touches)" :open="open ?? false" :expandable="state.touches.length > 0" data-test="decoy-connections" @update:open="open = $event">
          <template #aside><StatusTag v-if="state.touches.length" state="error" :label="DECOYS_NEEDS_LOOK" data-test="decoy-alert-tag" /></template>
          <ul class="m-0 flex list-none flex-col gap-1 p-0" aria-label="Devices that connected to a decoy" data-test="decoy-touches">
            <li v-for="t in state.touches" :key="`${t.source}-${t.port}`" class="flex flex-wrap justify-between gap-x-3">
              <span class="font-mono text-primary">{{ t.source }}</span>
              <span class="text-[length:var(--text-small)] text-muted">port {{ t.port }} · {{ t.count }} {{ t.count === 1 ? "time" : "times" }} · last {{ formatRelativeTime(t.last) }}</span>
            </li>
          </ul>
          <BaseButton v-if="state.touches.length" variant="secondary" size="sm" class-name="mt-2 max-sm:!h-11" :disabled="busy" data-test="decoy-clear" @click="clearList">{{ DECOYS_CLEAR_LABEL }}</BaseButton>
        </Disclosure>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ DECOYS_SCANNER_NOTE }}</p>

        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
      </template>
    </div>
  </Panel>
</template>
