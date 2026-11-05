<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import StatusTag from "../common/display/StatusTag.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useAgentActivity } from "../../composables/security/useAgentActivity"
import { agentEventSource, agentEventText, automationLine } from "../../utils/security/agentWatch"
import { formatAbsoluteTime, formatRelativeTime } from "../../utils/format/time"
import {
  AGENT_ACTIVITY_EMPTY, AGENT_ACTIVITY_EMPTY_BODY, AGENT_ACTIVITY_INTRO, AGENT_ACTIVITY_SHOWN, AGENT_ACTIVITY_TITLE, DESTINATIONS_TITLE,
  LOCKDOWN_CONFIRM_MESSAGE, LOCKDOWN_CONFIRM_TITLE, LOCKDOWN_ENGAGE, LOCKDOWN_OFF, LOCKDOWN_ON_TAG, LOCKDOWN_RELEASE, LOCKDOWN_RELEASE_MESSAGE,
  LOCKDOWN_RELEASE_TITLE, LOCKDOWN_REVOKE_HINT, LOCKDOWN_REVOKE_LABEL, LOCKDOWN_TITLE,
} from "../../constants/security"

// Settings · Safety · Agent activity: the lock down switch first (the one action), then the record as one quiet list, newest
// first, the latest few shown and the rest one tap away. Each row is one line of what happened and one small line of where
// and when, so a phone reads it without scrolling past noise.
const { confirm } = useConfirm()
const a = useAgentActivity()
const revokeTokens = ref(false)
const showAll = ref(false)
const destinationsOpen = ref(false)

const events = computed(() => a.record.value?.events ?? [])
const shown = computed(() => (showAll.value ? events.value : events.value.slice(0, AGENT_ACTIVITY_SHOWN)))
const automations = computed(() => a.record.value?.automations ?? [])
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
  <div class="flex flex-col gap-4" data-test="agent-activity-settings">
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

    <Panel :title="AGENT_ACTIVITY_TITLE">
      <div class="flex flex-col gap-3" data-test="agent-activity">
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ AGENT_ACTIVITY_INTRO }}</p>
        <ErrorState v-if="a.error.value && !a.record.value" title="The agent activity could not be read" :cause="a.error.value" next="Check that the server is running, then retry." />
        <LoadingState v-else-if="!a.record.value" label="Reading the agent activity" />
        <EmptyState v-else-if="!events.length" :title="AGENT_ACTIVITY_EMPTY" :body="AGENT_ACTIVITY_EMPTY_BODY" />
        <template v-else>
          <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
            <li v-for="(e, i) in shown" :key="`${e.at}-${i}`" class="flex flex-col gap-0.5 py-2 first:pt-0" data-test="agent-event" :data-kind="e.kind">
              <span class="break-words text-primary">{{ agentEventText(e) }}</span>
              <span class="flex flex-wrap gap-x-2 text-[length:var(--text-small)] text-muted">
                <span class="break-words">{{ agentEventSource(e) }}</span>
                <time :datetime="e.at" :title="formatAbsoluteTime(e.at)">{{ formatRelativeTime(e.at) }}</time>
              </span>
            </li>
          </ul>
          <div v-if="events.length > AGENT_ACTIVITY_SHOWN">
            <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="agent-show-all" @click="showAll = !showAll">
              {{ showAll ? "Show fewer" : `Show all ${events.length}` }}
            </BaseButton>
          </div>
        </template>
        <Disclosure v-if="automations.length" :title="DESTINATIONS_TITLE" :summary="String(automations.length)" :open="destinationsOpen" data-test="agent-destinations" @update:open="destinationsOpen = $event">
          <p v-for="d in automations" :key="`${d.workspace}/${d.automation}`" class="m-0 break-words text-[length:var(--text-small)] text-secondary">{{ automationLine(d) }}</p>
        </Disclosure>
      </div>
    </Panel>
  </div>
</template>
