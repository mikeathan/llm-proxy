<script setup lang="ts">
import { computed } from "vue"
import { useRoute } from "vue-router"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useAskPanel } from "../../composables/ui/useAskPanel"
import { useLastWorkspace } from "../../composables/ui/useLastWorkspace"
import { ROUTE_NAMES } from "../../types/routes"

// The header's way into the assistant from any page: a compact field that opens the
// ask panel. Absent where it would ask a second chat to open (the Assistant page is
// the assistant) and while no workspace is known. On a narrow screen, and while the
// panel is open (it has the room then), it is an icon.
const props = defineProps<{ mobile: boolean }>()

const { open, toggle } = useAskPanel()
const { lastWorkspace } = useLastWorkspace()
const route = useRoute()

const compact = computed(() => props.mobile || open.value)
const available = computed(() => !!lastWorkspace.value && route.name !== ROUTE_NAMES.workspaceAssistant)
</script>

<template>
  <BaseButton
    v-if="available"
    variant="secondary"
    icon="chat"
    :icon-only="compact"
    label="Ask the assistant"
    aria-controls="ask-panel"
    :aria-expanded="open"
    :class-name="compact ? '' : 'w-56 !justify-start !font-normal !text-muted hover:enabled:!text-primary'"
    @click="toggle"
  >Ask the agent…</BaseButton>
</template>
