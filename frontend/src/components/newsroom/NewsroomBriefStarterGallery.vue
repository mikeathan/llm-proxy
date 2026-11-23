<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { starterEdit, starterFeeds, usableStarters } from "../../utils/newsroom/briefForm"
import { BRIEF_STARTERS, STARTERS_ALL_LABEL, STARTERS_HINT, STARTERS_OWN_LABEL, STARTERS_TITLE, startersAddWords } from "../../constants/newsroom"
import type { BriefSource, BriefStarter, BriefTopicEdit } from "../../types/newsroom"

// Ready-made topics for an empty page: tap the ones you want and add them together, or make your own. It is shown only while there are no topics. The page does the saving (`add` gets the topics to save) and says if it fails.
const props = defineProps<{ sources: readonly BriefSource[]; saving: boolean }>()
const emit = defineEmits<{ (e: "add", edits: BriefTopicEdit[]): void; (e: "own"): void }>()

const starters = computed(() => usableStarters(BRIEF_STARTERS, props.sources))
const sections = computed(() => [...new Set(starters.value.map((s) => s.section))])
const section = ref("")
const shown = computed(() => starters.value.filter((s) => section.value === "" || s.section === section.value))
const picked = ref<string[]>([])
const toggle = (s: BriefStarter) => {
  picked.value = picked.value.includes(s.name) ? picked.value.filter((n) => n !== s.name) : [...picked.value, s.name]
}
const note = (s: BriefStarter): string => [...s.queries, ...starterFeeds(s, props.sources).map((x) => x.label)].join(", ")
const submit = () => emit("add", starters.value.filter((s) => picked.value.includes(s.name)).map((s) => starterEdit(s, props.sources)))
</script>

<template>
  <div class="flex flex-col gap-3" data-test="starter-gallery">
    <div>
      <h3 class="m-0 text-[length:var(--text-body)] font-medium text-primary">{{ STARTERS_TITLE }}</h3>
      <p class="m-0 text-[length:var(--text-small)] text-faint">{{ STARTERS_HINT }}</p>
    </div>
    <div class="flex flex-wrap gap-1.5" role="group" aria-label="Sections">
      <BaseButton v-for="s in ['', ...sections]" :key="s" size="sm" :variant="section === s ? 'secondary' : 'ghost'" :aria-pressed="section === s" @click="section = s">{{ s || STARTERS_ALL_LABEL }}</BaseButton>
    </div>
    <ul class="m-0 grid list-none gap-2 p-0 sm:grid-cols-2" aria-label="Starter topics">
      <li v-for="s in shown" :key="s.name">
        <button
          type="button"
          :aria-pressed="picked.includes(s.name)"
          :class="[
            'flex h-full w-full flex-col gap-0.5 rounded-[var(--radius-md)] border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2',
            picked.includes(s.name) ? 'border-accent-brand bg-surface-active' : 'border-hairline bg-canvas hover:border-accent-brand/60',
          ]"
          @click="toggle(s)"
        >
          <span class="font-medium text-primary">{{ picked.includes(s.name) ? "✓ " : "" }}{{ s.name }}</span>
          <span class="text-[length:var(--text-small)] text-faint">{{ s.section }} · {{ note(s) }}</span>
        </button>
      </li>
    </ul>
    <div class="flex flex-wrap items-center gap-2">
      <BaseButton size="sm" :disabled="saving || picked.length === 0" @click="submit">{{ startersAddWords(picked.length) }}</BaseButton>
      <BaseButton variant="ghost" size="sm" @click="emit('own')">{{ STARTERS_OWN_LABEL }}</BaseButton>
    </div>
  </div>
</template>
