<script setup lang="ts">
import { computed } from "vue"
import type { PromptProfile } from "../../types/profile"
import BaseButton from "../common/buttons/BaseButton.vue"
import MicroLabel from "../common/display/MicroLabel.vue"

// The agents to open, the user's own first, then the built-in ones. Each row says what the agent is for and what it is
// the default for, so they can be told apart without opening them.
const DEFAULT_FOR_LABEL: Record<string, string> = { assistant: "Default for chats", automation: "Default for automations" }
const YOURS = "Yours"
const BUILT_IN = "Built-in"

const props = defineProps<{ profiles: PromptProfile[]; selectedId: string; creating: boolean }>()
defineEmits<{ (e: "select", id: string): void; (e: "new"): void }>()

const groups = computed(() =>
  [
    { title: YOURS, items: props.profiles.filter((p) => p.source === "user") },
    { title: BUILT_IN, items: props.profiles.filter((p) => p.source !== "user") },
  ].filter((group) => group.items.length > 0),
)

function facts(profile: PromptProfile): string[] {
  const out = profile.default_for.map((kind) => DEFAULT_FOR_LABEL[kind] ?? kind)
  const helpers = profile.delegates?.length ?? 0
  if (helpers > 0) out.push(helpers === 1 ? "1 helper" : `${helpers} helpers`)
  return out
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <BaseButton variant="primary" icon="plus" :disabled="creating" class-name="w-full sm:w-auto" @click="$emit('new')">New agent</BaseButton>
    <section v-for="group in groups" :key="group.title" class="flex flex-col gap-1.5">
      <MicroLabel>{{ group.title }}</MicroLabel>
      <ul class="m-0 flex list-none flex-col gap-1 p-0">
        <li v-for="profile in group.items" :key="profile.id">
          <button
            type="button"
            :aria-current="profile.id === selectedId && !creating ? 'true' : undefined"
            :class="[
              'flex min-h-11 w-full flex-col items-start gap-0.5 rounded-[var(--radius-md)] px-3 py-2 text-left hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2',
              profile.id === selectedId && !creating ? 'bg-surface-raised' : '',
            ]"
            @click="$emit('select', profile.id)"
          >
            <span class="w-full truncate text-[length:var(--text-body)] font-medium text-primary">{{ profile.name }}</span>
            <span v-if="profile.summary" class="line-clamp-2 w-full text-[length:var(--text-small)] text-muted">{{ profile.summary }}</span>
            <span v-if="facts(profile).length" class="text-[length:var(--text-small)] text-faint">{{ facts(profile).join(" · ") }}</span>
          </button>
        </li>
      </ul>
    </section>
  </div>
</template>
