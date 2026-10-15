<script setup lang="ts">
import { computed, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import { useActiveWorkspace } from "../../composables/ui/useActiveWorkspace"
import { useDismissable } from "../../composables/ui/useDismissable"
import { toSameSectionIn, toWorkspaces, workspaceLocation } from "../../router/routes"
import Icon from "../icons/Icon.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import PopoverPanel from "../common/layout/PopoverPanel.vue"

// The workspace chip in the top strip (plan frontend-redesign-v2, V1): which workspace
// the Assistant, Workspace and the ask panel open. Choosing one remembers it and, on a
// workspace page, moves to the same section of the other workspace. A page that is
// about everything (Settings) says "All workspaces" and offers no choice. Creating and
// deleting workspaces stays on the Workspaces page, one link away.
defineProps<{ mobile: boolean }>()

const route = useRoute()
const router = useRouter()
const { workspaces, active, select } = useActiveWorkspace()

const open = ref(false)
const root = ref<HTMLElement | null>(null)
useDismissable(root, open)

const allWorkspaces = computed(() => route.meta.workspaceScope === "all")
const onWorkspacePage = computed(() => route.meta.destination === "workspaces" && typeof route.params.ws === "string")

function choose(id: string) {
  select(id)
  open.value = false
  if (onWorkspacePage.value && id !== route.params.ws) void router.push(toSameSectionIn(workspaceLocation(route), id))
}
</script>

<template>
  <span v-if="allWorkspaces" class="chip text-muted">
    <MicroLabel class="max-md:hidden">Workspace</MicroLabel>
    <span class="max-sm:hidden">All workspaces</span><span class="sm:hidden">All</span>
  </span>

  <div v-else-if="active" ref="root" class="relative flex-none">
    <button
      type="button"
      aria-haspopup="menu"
      aria-controls="workspace-menu"
      :aria-expanded="open"
      :aria-label="`Workspace: ${active}. Change workspace`"
      class="chip text-primary transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2"
      @click="open = !open"
    >
      <MicroLabel class="max-md:hidden">Workspace</MicroLabel>
      <span class="max-w-[10rem] truncate">{{ active }}</span>
      <Icon name="chevron-down" size="xs" />
    </button>

    <PopoverPanel v-if="open" id="workspace-menu" title="Workspace" width="sm:w-[280px]" align="left" @close="open = false">
      <div role="menu" aria-label="Workspaces" class="flex max-h-[50vh] flex-col gap-0.5 overflow-y-auto p-1.5">
        <button
          v-for="workspace in workspaces"
          :key="workspace.id"
          type="button"
          role="menuitemradio"
          :aria-checked="workspace.id === active"
          :aria-current="workspace.id === active ? 'true' : undefined"
          :class="['choice', { 'choice--current': workspace.id === active }]"
          @click="choose(workspace.id)"
        >{{ workspace.id }}</button>
      </div>
      <RouterLink
        :to="toWorkspaces()"
        class="px-3 py-2.5 font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2"
        @click="open = false"
      >Manage workspaces</RouterLink>
    </PopoverPanel>
  </div>
</template>

<style scoped>
.chip { @apply inline-flex h-[30px] flex-none items-center gap-2 rounded-[var(--radius-md)] bg-surface-raised px-2.5 font-mono text-[length:var(--text-small)]; }
.choice { @apply flex min-h-10 w-full items-center rounded-[var(--radius-md)] px-2.5 text-left font-mono text-[length:var(--text-small)] text-secondary hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2; }
.choice--current { @apply bg-surface-active text-primary; box-shadow: inset 2px 0 0 rgb(var(--accent-brand)); }
</style>
