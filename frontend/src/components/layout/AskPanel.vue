<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue"
import { useRoute } from "vue-router"
import AssistantChat from "../AgentIde/assistant/AssistantChat.vue"
import { openConversationId } from "../../composables/assistant/useAssistant"
import { useAskPanel } from "../../composables/ui/useAskPanel"
import { useLastWorkspace } from "../../composables/ui/useLastWorkspace"
import { useResponsiveLayout } from "../../composables/ui/useResponsiveLayout"
import { ROUTE_NAMES } from "../../types/routes"

// The assistant beside any page (plan frontend-redesign-v2, V2): a column on a wide
// screen, a full-screen dialog on a narrow one. It talks to the workspace last used
// and resumes the conversation that is open (the run itself lives on the server, so
// one started on the Assistant page carries on here).
//
// Exactly one AssistantChat may be live — each useAssistant() owns a connection and a
// message builder, and two would feed the same messages. This panel therefore opens
// only on pages that have no chat of their own: the Assistant page's chat is the only
// other one, the panel gives way to it, and Workspaces closes its chat when it is left.
const { open, hide } = useAskPanel()
const { lastWorkspace } = useLastWorkspace()
const { isMobile } = useResponsiveLayout()
const route = useRoute()

const onAssistantPage = computed(() => route.name === ROUTE_NAMES.workspaceAssistant)
const visible = computed(() => open.value && !!lastWorkspace.value && !onAssistantPage.value)

// The conversation to resume is the one open when the panel opens.
const resumeId = ref<string | null>(null)
const chat = ref<InstanceType<typeof AssistantChat> | null>(null)
let opener: HTMLElement | null = null

// On the Assistant page the panel is never open, whichever way it got asked to be.
watch([open, onAssistantPage], ([isOpen, onPage]) => {
  if (isOpen && onPage) hide()
})

watch(
  visible,
  async (shown) => {
    if (shown) {
      resumeId.value = openConversationId.value
      opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
      await nextTick()
      chat.value?.focusInput()
    } else {
      opener?.focus()
      opener = null
    }
  },
  { immediate: true },
)
</script>

<template>
  <aside
    v-if="visible && lastWorkspace"
    id="ask-panel"
    :role="isMobile ? 'dialog' : 'complementary'"
    :aria-modal="isMobile ? 'true' : undefined"
    aria-label="Assistant"
    :class="isMobile ? 'fixed inset-0 z-50 bg-canvas' : 'sticky top-0 h-screen w-[28rem] max-w-[45vw] flex-none bg-surface-raised'"
    @keydown.escape="hide"
  >
    <AssistantChat ref="chat" variant="panel" :workspace-id="lastWorkspace" :conversation-id="resumeId" @close="hide" />
  </aside>
</template>
