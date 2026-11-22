<script setup lang="ts">
import { computed } from "vue"
import { groupSources } from "../../utils/newsroom/briefForm"
import { SOURCES_BROWSE_HINT, SOURCES_BROWSE_LABEL, SOURCE_ADDED_LABEL, SOURCE_ADD_LABEL, SOURCE_SEARCH_NOTE } from "../../constants/newsroom"
import type { BriefSource } from "../../types/newsroom"

// The well-known sources and the owner's own, by group, to add to a topic with one press. A group that matches the topic's name comes first. A feed is
// followed and a publisher search is added to the topic's searches; the editor does the adding and says which are already in (`chosen`).
const props = defineProps<{ sources: readonly BriefSource[]; order: readonly string[]; topicName: string; chosen: (s: BriefSource) => boolean }>()
const emit = defineEmits<{ (e: "toggle", source: BriefSource): void }>()
const groups = computed(() => groupSources(props.sources, props.topicName, props.order))
</script>

<template>
  <details v-if="sources.length" class="rounded-[var(--radius-md)] border border-hairline" data-test="source-browser">
    <summary class="cursor-pointer select-none px-3 py-2 text-[length:var(--text-small)] font-medium text-secondary">{{ SOURCES_BROWSE_LABEL }}</summary>
    <div class="flex flex-col gap-3 border-t border-hairline p-3">
      <p class="m-0 text-[length:var(--text-small)] text-faint">{{ SOURCES_BROWSE_HINT }}</p>
      <section v-for="g in groups" :key="g.category" :aria-label="g.category" class="flex flex-col gap-1.5">
        <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ g.category }}</h3>
        <ul class="m-0 flex list-none flex-wrap gap-1.5 p-0">
          <li v-for="s in g.entries" :key="s.kind + s.target">
            <button
              type="button"
              :aria-pressed="chosen(s)"
              :title="s.kind === 'newssearch' ? `${s.label}: ${SOURCE_SEARCH_NOTE}` : s.target"
              :class="[
                'inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[length:var(--text-small)] transition-colors focus-visible:outline-none focus-visible:ring-2',
                chosen(s) ? 'border-accent-brand bg-surface-active text-primary' : 'border-control bg-canvas text-secondary hover:border-accent-brand/60 hover:text-primary',
              ]"
              @click="emit('toggle', s)"
            >
              <span>{{ s.label }}</span>
              <span class="sr-only">{{ chosen(s) ? SOURCE_ADDED_LABEL : SOURCE_ADD_LABEL }}</span>
              <span aria-hidden="true" class="text-faint">{{ chosen(s) ? "✓" : "+" }}</span>
            </button>
          </li>
        </ul>
      </section>
    </div>
  </details>
</template>
