<script setup lang="ts">
import { computed, ref, watch } from "vue"
import { RouterLink } from "vue-router"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import EmptyState from "../../common/feedback/EmptyState.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import { CARD_LIST_SCROLL } from "../../../constants/dashboard"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useActiveWorkspace } from "../../../composables/ui/useActiveWorkspace"
import { useLastWorkspace } from "../../../composables/ui/useLastWorkspace"
import { usePolling } from "../../../composables/ui/usePolling"
import { AssistantService } from "../../../services/assistant/assistantService"
import { toWorkspaceAssistant } from "../../../router/routes"
import { formatRelativeTime } from "../../../utils/format/time"
import { errorMessage } from "../../../utils/errors"
import type { SessionBrief } from "../../../types/assistant"

// Dashboard card · Recent conversations: the latest conversations of the workspace the assistant opens, newest first, each a link into it. A
// conversation still running says so. Small is the count; medium and large show a list that scrolls, so every conversation (up to KEPT) is in
// it and a smaller card never hides the rest. It reads; talking is in the assistant.
const KEPT = 50
const REFRESH_MS = 30000
const FAILED = "The conversations could not be read."
const size = useCardSize()
const { lastWorkspace } = useLastWorkspace()
const workspace = computed(() => lastWorkspace.value)
const { workspaces } = useActiveWorkspace()
const sessions = ref<SessionBrief[] | null>(null)
const error = ref("")
let latest = 0

async function load(): Promise<void> {
  const ws = workspace.value
  if (!ws) return
  const mine = ++latest
  try {
    const list = await AssistantService.listSessions(ws)
    if (mine !== latest) return
    sessions.value = [...list].sort((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at))
    error.value = ""
  } catch (err) {
    if (mine === latest) error.value = errorMessage(err, FAILED)
  }
}

const shown = computed(() => (size.value === 'xs' ? [] : (sessions.value ?? []).slice(0, KEPT)))
const running = computed(() => (sessions.value ?? []).filter((s) => s.running).length)
watch(workspace, () => { sessions.value = null; void load() }, { immediate: true })
usePolling(() => { void load() }, REFRESH_MS)
</script>

<template>
  <div data-test="assistant-recent-card">
    <EmptyState v-if="!workspace && !workspaces.length" title="No workspace yet" body="Make a workspace to start a conversation." />
    <ErrorState v-else-if="error && !sessions" title="The conversations could not be read" :cause="error" next="It is read again in a minute." />
    <LoadingState v-else-if="!sessions" label="Reading the conversations" :rows="2" />
    <EmptyState v-else-if="!sessions.length" title="No conversations yet" body="Start one from the assistant." />
    <template v-else>
      <p class="m-0 text-primary" data-test="assistant-recent-count">{{ sessions.length }} {{ sessions.length === 1 ? "conversation" : "conversations" }}<template v-if="running"> · {{ running }} running</template></p>
      <ul v-if="shown.length" :class="['m-0 mt-2 flex list-none flex-col p-0', CARD_LIST_SCROLL[size]]" aria-label="Recent conversations" data-test="assistant-recent-list">
        <li v-for="s in shown" :key="s.id" class="border-b border-hairline/60 last:border-b-0" data-test="assistant-recent-session">
          <RouterLink :to="toWorkspaceAssistant(workspace!, s.id)" class="flex min-h-11 flex-col gap-0.5 py-2 hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2">
            <span class="flex flex-wrap items-center gap-x-2 gap-y-1">
              <span class="min-w-0 break-words text-primary">{{ s.snippet || "Empty conversation" }}</span>
              <StatusTag v-if="s.running" state="running" label="Running" />
            </span>
            <span class="text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(s.updated_at) }}</span>
          </RouterLink>
        </li>
      </ul>
    </template>
  </div>
</template>
