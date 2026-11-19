<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import StatusTag from "../common/display/StatusTag.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useAgentActivity } from "../../composables/security/useAgentActivity"
import { formatAbsoluteTime } from "../../utils/format/time"
import {
  LOCKDOWN_CONFIRM_MESSAGE, LOCKDOWN_CONFIRM_TITLE, LOCKDOWN_ENGAGE, LOCKDOWN_OFF, LOCKDOWN_ON_TAG, LOCKDOWN_RELEASE, LOCKDOWN_RELEASE_MESSAGE,
  LOCKDOWN_RELEASE_TITLE, LOCKDOWN_REVOKE_HINT, LOCKDOWN_REVOKE_LABEL, LOCKDOWN_TITLE,
} from "../../constants/security"

// Agents · Lock down: the stack's one action, kept on the page because it ends sessions and needs a confirmation. The record of what the agents
// did is the Agent activity card on the dashboard's Agents board.
const { confirm } = useConfirm()
const a = useAgentActivity()
const revokeTokens = ref(false)

const endedLine = computed(() => {
  const l = a.lockdown.value
  if (!l?.on) return ""
  const parts = [l.signed_out ? `${l.signed_out} other ${l.signed_out === 1 ? "session was" : "sessions were"} signed out` : "", l.tokens_revoked ? `${l.tokens_revoked} ${l.tokens_revoked === 1 ? "token was" : "tokens were"} revoked` : ""]
  return parts.filter(Boolean).join(" · ")
})

async function engage() {
  const ok = await confirm({ title: LOCKDOWN_CONFIRM_TITLE, message: LOCKDOWN_CONFIRM_MESSAGE, confirmText: LOCKDOWN_ENGAGE, type: "warning" })
  if (ok) await a.engage(revokeTokens.value)
}

async function release() {
  const ok = await confirm({ title: LOCKDOWN_RELEASE_TITLE, message: LOCKDOWN_RELEASE_MESSAGE, confirmText: LOCKDOWN_RELEASE, type: "warning" })
  if (ok) await a.release()
}

onMounted(() => { void a.load() })
</script>

<template>
  <div class="flex flex-col gap-4" data-test="agent-lockdown-panel">
    <Panel :title="LOCKDOWN_TITLE">
      <div class="flex flex-col gap-3" data-test="lockdown">
        <template v-if="a.lockdown.value?.on">
          <p class="m-0 flex flex-wrap items-center gap-2 text-primary">
            <StatusTag state="error" :label="LOCKDOWN_ON_TAG" />
            <span data-test="lockdown-since">since {{ formatAbsoluteTime(a.lockdown.value.since ?? "") }}</span>
          </p>
          <p v-if="endedLine" class="m-0 text-[length:var(--text-small)] text-muted">{{ endedLine }}</p>
          <div><BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="a.busy.value" data-test="lockdown-release" @click="release">{{ LOCKDOWN_RELEASE }}</BaseButton></div>
        </template>
        <template v-else-if="a.lockdown.value">
          <p class="m-0 text-secondary">{{ LOCKDOWN_OFF }}</p>
          <ToggleField v-model="revokeTokens" :label="LOCKDOWN_REVOKE_LABEL" :hint="LOCKDOWN_REVOKE_HINT" />
          <div><BaseButton variant="danger" class-name="max-sm:!h-11" :disabled="a.busy.value" data-test="lockdown-engage" @click="engage">{{ LOCKDOWN_ENGAGE }}</BaseButton></div>
        </template>
        <LoadingState v-else-if="a.loading.value" label="Reading the lock down switch" />
        <p v-if="a.lockdownError.value" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ a.lockdownError.value }}</p>
      </div>
    </Panel>
  </div>
</template>
