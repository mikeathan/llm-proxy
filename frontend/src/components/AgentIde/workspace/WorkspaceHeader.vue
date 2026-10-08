<script setup lang="ts">
import type { RouteLocationNamedRaw } from "vue-router"
import { toWorkspaceAssistant, toWorkspaceFile, toWorkspaceSection } from "../../../router/routes"
import type { WorkspaceNavSection } from "../../../types/routes"
import SelectInput from "../../common/forms/SelectInput.vue"
import StatusTag from "../../common/display/StatusTag.vue"

// A workspace page's header: switch workspace, and move between its sections
// (real links, so each section is addressable and back/forward works).

const props = defineProps<{
  ws: string
  workspaces: { id: string }[]
  section: WorkspaceNavSection
  chatRunning: boolean
  externalAccess: boolean
}>()

const emit = defineEmits<{ (e: "switch", id: string): void }>()
defineSlots<{ actions?(): unknown }>()

interface SectionLink {
  id: WorkspaceNavSection
  label: string
  to: (ws: string) => RouteLocationNamedRaw
}

const SECTIONS: SectionLink[] = [
  { id: "files", label: "Files", to: (ws) => toWorkspaceFile(ws) },
  { id: "assistant", label: "Assistant", to: (ws) => toWorkspaceAssistant(ws) },
  { id: "memory", label: "Memory", to: (ws) => toWorkspaceSection(ws, "memory") },
  { id: "playbooks", label: "Playbooks", to: (ws) => toWorkspaceSection(ws, "playbooks") },
  { id: "heartbeat", label: "Heartbeat", to: (ws) => toWorkspaceSection(ws, "heartbeat") },
  { id: "settings", label: "Settings", to: (ws) => toWorkspaceSection(ws, "settings") },
]
</script>

<template>
  <div class="flex flex-wrap items-center gap-3">
    <SelectInput
      :model-value="props.ws"
      :options="props.workspaces.map((w) => ({ value: w.id, label: w.id }))"
      label="Workspace"
      @update:model-value="emit('switch', $event)"
    />
    <nav aria-label="Workspace sections" class="inline-flex max-w-full overflow-x-auto rounded-[var(--radius-md)] border border-control">
      <RouterLink
        v-for="(link, index) in SECTIONS"
        :key="link.id"
        :to="link.to(props.ws)"
        :aria-current="link.id === props.section ? 'page' : undefined"
        :class="[
          'inline-flex h-7 flex-none items-center gap-1.5 whitespace-nowrap px-3 font-mono text-[length:var(--text-small)] font-medium transition-colors duration-fast focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset',
          index > 0 ? 'border-l border-hairline' : '',
          link.id === props.section ? 'bg-surface-active text-primary' : 'text-muted hover:bg-surface-hover hover:text-primary',
        ]"
      >
        {{ link.label }}
        <span v-if="link.id === 'assistant' && props.chatRunning" class="inline-flex items-center gap-1 text-state-running">
          <span aria-hidden="true" class="h-1.5 w-1.5 rounded-full bg-state-live"></span>
          <span class="sr-only">running</span>
        </span>
      </RouterLink>
    </nav>
    <StatusTag v-if="props.externalAccess" state="info" label="External access" />
    <div v-if="$slots.actions" class="ml-auto flex items-center gap-2"><slot name="actions" /></div>
  </div>
</template>
