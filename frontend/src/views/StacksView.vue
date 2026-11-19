<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import { StacksService } from "../services/stacks/stacksService"
import { useEnabledStacks } from "../composables/stacks/useEnabledStacks"
import { errorMessage } from "../utils/errors"
import { toAgents, toNetwork, toRadio, toReminders } from "../router/routes"
import { STACK_LOAD_FAILED, STACKS_GROUP_LABEL, STACKS_INTRO, STACKS_MODULES_LINK, stackStateLine } from "../constants/stacks"
import type { StackId, StackSummary } from "../types/stacks"

// Stacks: the one list of every stack and whether anything of it is on. A stack with no module on has no entry in the menu and no board on the
// dashboard; its Modules page is where it is switched on, and this page is the way there.
const stacks = ref<StackSummary[] | null>(null)
const error = ref("")
const enabled = useEnabledStacks()
const MODULES_PAGE: Record<StackId, () => ReturnType<typeof toNetwork>> = {
  network: () => toNetwork("modules"),
  agents: () => toAgents("modules"),
  radio: () => toRadio("modules"),
  reminders: () => toReminders("modules"),
}
const rows = computed(() => (stacks.value ?? []).map((s) => ({ ...s, to: MODULES_PAGE[s.id]() })))

onMounted(async () => {
  try {
    stacks.value = await StacksService.list()
  } catch (err) {
    error.value = errorMessage(err, STACK_LOAD_FAILED)
  }
  void enabled.load()
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="STACKS_GROUP_LABEL" :subtitle="STACKS_INTRO" subtitle-hidden-on-phone />
    <p v-if="error" role="alert" class="m-0 text-state-error">{{ error }}</p>
    <LoadingState v-else-if="!stacks" label="Reading the stacks" :rows="3" />
    <Panel v-for="s in rows" v-else :key="s.id" :title="s.title">
      <div class="flex flex-col gap-2" :data-test="`stack-${s.id}`">
        <p class="m-0 text-secondary">{{ s.purpose }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ stackStateLine(s.on, s.total) }}</p>
        <RouterLink :to="s.to" class="text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2 max-sm:inline-flex max-sm:min-h-11 max-sm:items-center">{{ STACKS_MODULES_LINK }} →</RouterLink>
      </div>
    </Panel>
  </div>
</template>
