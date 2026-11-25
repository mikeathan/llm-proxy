<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { storyLessHint, storyMoreHint, storyTuneApply } from "../../constants/newsroom"

// "More like this" or "Less like this" on one story: the story's own words as toggles, the first two picked, and one button that changes the topic the
// story came from. Nothing is guessed: the owner sees and picks the words that will be searched for or skipped.
const props = defineProps<{ words: string[]; more: boolean; topic: string }>()
const emit = defineEmits<{ (e: "apply", words: string[]): void; (e: "cancel"): void }>()
const picked = ref<string[]>(props.words.slice(0, 2))
const flip = (w: string) => (picked.value = picked.value.includes(w) ? picked.value.filter((x) => x !== w) : [...picked.value, w])
</script>

<template>
  <div class="flex flex-col gap-2 border-l-2 border-accent-brand bg-surface-raised px-3 py-2 print:hidden" data-test="story-tune">
    <p class="m-0 text-[length:var(--text-small)] text-secondary">{{ more ? storyMoreHint(topic) : storyLessHint(topic) }}</p>
    <div class="flex flex-wrap gap-1.5" role="group" :aria-label="more ? storyMoreHint(topic) : storyLessHint(topic)">
      <button
        v-for="w in words"
        :key="w"
        type="button"
        :aria-pressed="picked.includes(w)"
        :class="['inline-flex min-h-11 items-center rounded-[var(--radius-sm)] border px-2.5 font-mono text-[length:var(--text-small)] focus-visible:outline-none focus-visible:ring-2 sm:min-h-8', picked.includes(w) ? 'border-accent-brand bg-surface-active text-primary' : 'border-hairline text-muted hover:bg-surface-hover']"
        data-test="tune-word"
        @click="flip(w)"
      >{{ w }}</button>
    </div>
    <div class="flex gap-2">
      <BaseButton size="sm" :disabled="!picked.length" data-test="tune-apply" @click="emit('apply', picked)">{{ storyTuneApply(more) }}</BaseButton>
      <BaseButton size="sm" variant="ghost" @click="emit('cancel')">Cancel</BaseButton>
    </div>
  </div>
</template>
