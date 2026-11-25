<script setup lang="ts">
import BaseButton from "../common/buttons/BaseButton.vue"
import { EXAMPLES_HINT, EXAMPLES_KIND_LABELS, EXAMPLES_LABEL, removeExampleLabel } from "../../constants/newsroom"
import type { NewsroomExamples } from "../../types/newsroom"

// What a watch learned from the owner: the posts they marked "Should have matched" (wanted) and "Not for me", newest first, each removable when it was
// marked by mistake. The lists are the form's to save; this only shows them and reports a removal.
const props = defineProps<{ examples: NewsroomExamples }>()
const emit = defineEmits<{ (e: "update:examples", examples: NewsroomExamples): void }>()
const KINDS = ["want", "skip"] as const
function remove(kind: (typeof KINDS)[number], i: number) {
  const list = [...(props.examples[kind] ?? [])]
  list.splice(i, 1)
  emit("update:examples", { ...props.examples, [kind]: list })
}
</script>

<template>
  <section class="flex flex-col gap-2" data-test="watch-examples" :aria-label="EXAMPLES_LABEL">
    <span class="text-[length:var(--text-small)] font-medium text-secondary">{{ EXAMPLES_LABEL }}</span>
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ EXAMPLES_HINT }}</p>
    <div class="grid gap-3 sm:grid-cols-2">
      <div v-for="kind in KINDS" :key="kind" class="flex min-w-0 flex-col gap-1">
        <template v-if="examples[kind]?.length">
          <span class="font-mono text-[length:var(--text-small)] uppercase tracking-wider text-faint">{{ EXAMPLES_KIND_LABELS[kind] }}</span>
          <ul class="m-0 flex list-none flex-col divide-y divide-hairline border-y border-hairline p-0">
            <li v-for="(title, i) in examples[kind]" :key="title" class="flex min-w-0 items-center gap-2 py-1">
              <span class="min-w-0 flex-1 break-words text-[length:var(--text-small)] text-primary">{{ title }}</span>
              <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="removeExampleLabel(title)" @click="remove(kind, i)" />
            </li>
          </ul>
        </template>
      </div>
    </div>
  </section>
</template>
