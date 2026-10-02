<script setup lang="ts">
import { onUnmounted, ref } from 'vue'
import { useMemory } from '../../../composables/memory/useMemory'
import type { MemoryImportResult } from '../../../types/memory'
import BaseButton from '../../common/buttons/BaseButton.vue'

// Back up, move or hand-edit memory as one markdown file: Export downloads the
// workspace's facts (and your user-wide ones); Import adds the facts of a file,
// skipping any already saved and listing entries it could not use.

const props = defineProps<{ workspaceId: string }>()
const emit = defineEmits<{ (e: 'imported'): void }>()

const { exportMemory, importMemory, error } = useMemory()

const ACCEPT = '.md,text/markdown,text/plain'
const MIME_MARKDOWN = 'text/markdown;charset=utf-8'

const input = ref<HTMLInputElement | null>(null)
const result = ref<MemoryImportResult | null>(null)
const failure = ref('')
const busy = ref(false)
let objectUrl: string | null = null
onUnmounted(() => {
  if (objectUrl) URL.revokeObjectURL(objectUrl)
})

async function download() {
  failure.value = ''
  const text = await exportMemory(props.workspaceId)
  if (text === null) {
    failure.value = error.value ?? 'Could not export memory.'
    return
  }
  if (objectUrl) URL.revokeObjectURL(objectUrl)
  objectUrl = URL.createObjectURL(new Blob([text], { type: MIME_MARKDOWN }))
  const link = document.createElement('a')
  link.href = objectUrl
  link.download = `memory-${props.workspaceId}.md`
  link.click()
}

async function onFile(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  busy.value = true
  failure.value = ''
  result.value = null
  const imported = await importMemory(props.workspaceId, await file.text())
  busy.value = false
  target.value = '' // so choosing the same file again re-runs the import
  if (!imported) {
    failure.value = error.value ?? 'Could not import this file.'
    return
  }
  result.value = imported
  if (imported.created > 0) emit('imported')
}

const summary = (r: MemoryImportResult) =>
  `Added ${r.created}${r.skipped ? `, ${r.skipped} already saved` : ''}${r.issues.length ? `, ${r.issues.length} could not be imported` : ''}.`
</script>

<template>
  <div class="flex flex-col gap-2">
    <div class="flex flex-wrap items-center gap-2">
      <BaseButton variant="ghost" size="sm" icon="arrow-down" @click="download">Export</BaseButton>
      <BaseButton variant="ghost" size="sm" icon="arrow-up" :loading="busy" @click="input?.click()">Import</BaseButton>
      <input ref="input" type="file" :accept="ACCEPT" class="sr-only" aria-label="Import memory file" tabindex="-1" @change="onFile" />
    </div>
    <p v-if="failure" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ failure }}</p>
    <div v-if="result" role="status" class="flex flex-col gap-1 text-[length:var(--text-small)] text-secondary">
      <p class="m-0">{{ summary(result) }}</p>
      <ul v-if="result.issues.length" class="m-0 flex list-none flex-col gap-0.5 p-0 text-state-running">
        <li v-for="issue in result.issues" :key="issue.line">Line {{ issue.line }}: {{ issue.message }}</li>
      </ul>
    </div>
  </div>
</template>
