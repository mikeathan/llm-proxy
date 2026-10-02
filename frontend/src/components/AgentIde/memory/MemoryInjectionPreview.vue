<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMemory } from '../../../composables/memory/useMemory'
import { useModels } from '../../../composables/models/useModels'
import { PRIORITY_HIGH } from '../../../constants/memory'
import type { ChoiceOption } from '../../../types/ui'
import SelectInput from '../../common/forms/SelectInput.vue'
import Callout from '../../common/feedback/Callout.vue'

// What a run of the chosen model would receive from memory: the exact block
// (built by the same code the agent runs), its size against the model's own
// context window, and which facts did not fit.

const props = defineProps<{ workspaceId: string }>()

const NO_MODEL = ''
const NO_MODEL_LABEL = 'No model (default budget)'

const { injectionPreview, fetchInjectionPreview, memoryRevision } = useMemory()
const { state } = useModels()

const modelOptions = computed<ChoiceOption[]>(() => [
  ...(state.value?.models ?? []).map((m) => ({ value: m.name, label: m.name })),
  { value: NO_MODEL, label: NO_MODEL_LABEL },
])
const model = ref(state.value?.models?.[0]?.name ?? NO_MODEL)

// Reload on a workspace or model change, and whenever memory changes underneath
// an open preview (a fact added, toggled, edited, deleted; notes saved).
watch([() => props.workspaceId, model, memoryRevision], () => fetchInjectionPreview(props.workspaceId, model.value), { immediate: true })

const preview = computed(() => injectionPreview.value)
const windowShare = computed(() => {
  const p = preview.value
  if (!p || p.context_budget_chars <= 0) return null
  return Math.round((p.chars / p.context_budget_chars) * 100)
})
const highPriorityCut = computed(() => preview.value?.cut.some((c) => c.priority === PRIORITY_HIGH) ?? false)
const formatNumber = (n: number) => n.toLocaleString('en-US')
</script>

<template>
  <section class="flex min-w-0 flex-col gap-3" aria-label="What the model receives">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <p class="m-0 text-[length:var(--text-small)] text-muted">What a run of this model receives from memory.</p>
      <SelectInput v-model="model" :options="modelOptions" label="Model to preview" />
    </div>

    <template v-if="preview">
      <Callout v-if="!preview.budget_resolved" tone="info" title="Default budget">
        No model is selected, so this uses a default budget rather than a model's own context window.
      </Callout>

      <Callout v-if="preview.over_budget" tone="warning" title="Operator notes alone use the whole budget">
        Operator notes alone are {{ formatNumber(preview.operator_chars) }} chars, over this model's {{ formatNumber(preview.budget_chars) }}-char
        memory budget. They are never cut, so saved facts give way. Shorten the notes or choose a larger model.
      </Callout>

      <p class="m-0 font-mono text-[length:var(--text-small)] text-secondary">
        ~{{ formatNumber(preview.tokens_estimate) }} tokens ·
        {{ formatNumber(preview.chars) }} of {{ formatNumber(preview.budget_chars) }} chars<template v-if="windowShare !== null"> · {{ windowShare }}% of the context window</template>
        <template v-if="preview.operator_chars > 0"> · {{ formatNumber(preview.operator_chars) }} of those chars are your notes</template>
      </p>

      <pre
        v-if="preview.block"
        data-test="injection-block"
        class="m-0 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-[var(--radius-sm)] border border-hairline bg-canvas p-3 font-mono text-[length:var(--text-small)] text-primary"
      >{{ preview.block }}</pre>
      <p v-else class="m-0 text-[length:var(--text-small)] text-muted">
        No always-on facts: nothing is added to the prompt.
      </p>

      <Callout v-if="highPriorityCut" tone="warning" title="A high-priority fact did not fit">
        Even the protected facts exceed this model's memory budget. Shorten them, shorten the operator notes, or choose a larger model.
      </Callout>

      <div v-if="preview.cut.length > 0" class="flex flex-col gap-1">
        <p class="m-0 text-[length:var(--text-small)] font-medium text-state-running">
          Not sent ({{ preview.cut.length }}) — over the budget:
        </p>
        <ul class="m-0 flex list-none flex-col gap-0.5 p-0 text-[length:var(--text-small)] text-muted">
          <li v-for="item in preview.cut" :key="item.id" class="truncate">{{ item.title || 'Untitled' }}</li>
        </ul>
      </div>
    </template>
  </section>
</template>
