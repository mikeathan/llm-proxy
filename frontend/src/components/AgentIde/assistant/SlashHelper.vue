<script setup lang="ts">
import type { CommandInfo, SlashPreview } from '../../../types/commands'

// Help while typing a slash command: the matching commands while only the name is typed, then the active command's
// usage, clickable examples and a live preview of what it would do.

defineProps<{
  suggestions: CommandInfo[]
  active: CommandInfo | null
  preview: SlashPreview | null
}>()

const emit = defineEmits<{
  pick: [command: CommandInfo]
  example: [command: CommandInfo, example: string]
}>()
</script>

<template>
  <div v-if="suggestions.length || active" class="slash-helper" data-test="slash-helper">
    <ul v-if="suggestions.length" class="m-0 flex list-none flex-col p-0" aria-label="Commands">
      <li v-for="c in suggestions" :key="c.name">
        <button type="button" class="slash-row" :data-test="`slash-command-${c.name}`" @click="emit('pick', c)">
          <span class="font-mono text-primary">/{{ c.name }}</span>
          <span class="text-muted">{{ c.summary }}</span>
        </button>
      </li>
    </ul>
    <div v-else-if="active" class="flex flex-col gap-1.5 px-3 py-2">
      <span class="font-mono text-[length:var(--text-small)] text-secondary" data-test="slash-usage">{{ active.usage }}</span>
      <p v-if="preview?.error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="slash-error">{{ preview.error }}</p>
      <p v-else-if="preview?.text" class="m-0 text-[length:var(--text-small)] text-primary" data-test="slash-preview">{{ preview.text }}</p>
      <div v-else class="flex flex-wrap gap-1.5">
        <button
          v-for="example in active.examples"
          :key="example"
          type="button"
          class="slash-example"
          data-test="slash-example"
          @click="emit('example', active, example)"
        >{{ example }}</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.slash-helper { @apply rounded-[var(--radius-md)] border border-hairline bg-surface text-[length:var(--text-small)]; }
.slash-row { @apply flex w-full items-baseline gap-3 px-3 py-2 text-left transition-colors hover:bg-canvas focus-visible:outline-none focus-visible:ring-2; }
.slash-example { @apply rounded-[var(--radius-sm)] border border-hairline bg-canvas px-2 py-1 font-mono text-[length:var(--text-micro)] text-secondary transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-2; }
</style>
