<script setup lang="ts">
import { onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useRouterLog } from "../../composables/netaudit/useRouterLog"
import {
  ROUTER_LOG_DETAILS, ROUTER_LOG_HINT, ROUTER_LOG_NOT_HEARD, ROUTER_LOG_SETUP_AFTER, ROUTER_LOG_SETUP_COMMAND, ROUTER_LOG_SETUP_STEPS,
  ROUTER_LOG_SETUP_TITLE, ROUTER_LOG_TITLE, ROUTER_LOG_UNDO_COMMAND,
} from "../../constants/netaudit"
import { formatRelativeTime } from "../../utils/format/time"

// Settings · Network · Router log: the switch for taking the router's log, whether it is arriving, and how to point the router
// at this computer. The log is only a nudge for the presence check, so there is nothing to list here.
const { state, error, busy, load, set } = useRouterLog()
const setupOpen = ref(false)

onMounted(load)
</script>

<template>
  <Panel :title="ROUTER_LOG_TITLE">
    <div class="flex flex-col gap-3" data-test="router-log">
      <ErrorState v-if="error && !state" title="The router log setting could not be read" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!state" label="Reading the router log setting" />

      <template v-else>
        <ToggleField :model-value="state.enabled" label="Take the router's log" :hint="ROUTER_LOG_HINT" :details="ROUTER_LOG_DETAILS" :disabled="busy" @update:model-value="set" />

        <p v-if="state.enabled && state.problem" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="router-log-problem">The program cannot listen on port {{ state.port }}: {{ state.problem }}</p>
        <p v-else-if="state.enabled && state.last_heard" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="router-log-heard">Listening on port {{ state.port }}. The router was last heard {{ formatRelativeTime(state.last_heard) }}.</p>
        <p v-else-if="state.enabled && state.listening" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="router-log-silent">Listening on port {{ state.port }}. {{ ROUTER_LOG_NOT_HEARD }}</p>

        <Disclosure :title="ROUTER_LOG_SETUP_TITLE" summary="Router setting and one command" :open="setupOpen" data-test="router-log-setup" @update:open="setupOpen = $event">
          <ol class="m-0 flex list-decimal flex-col gap-1 pl-5 text-[length:var(--text-small)] text-secondary">
            <li v-for="step in ROUTER_LOG_SETUP_STEPS" :key="step">{{ step }}</li>
          </ol>
          <pre class="m-0 mt-2 overflow-x-auto rounded-[var(--radius-md)] bg-surface-raised p-2 font-mono text-[length:var(--text-small)] text-primary" tabindex="0" data-test="router-log-command">{{ ROUTER_LOG_SETUP_COMMAND }}</pre>
          <p class="m-0 mt-2 text-[length:var(--text-small)] text-muted">{{ ROUTER_LOG_SETUP_AFTER }}</p>
          <pre class="m-0 mt-1 overflow-x-auto rounded-[var(--radius-md)] bg-surface-raised p-2 font-mono text-[length:var(--text-small)] text-primary" tabindex="0">{{ ROUTER_LOG_UNDO_COMMAND }}</pre>
        </Disclosure>

        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
      </template>
    </div>
  </Panel>
</template>
