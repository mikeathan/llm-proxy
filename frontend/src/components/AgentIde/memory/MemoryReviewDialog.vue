<script setup lang="ts">
import { MEMORY_RECALL_LABEL, MEMORY_SCOPE_LABEL } from '../../../constants/memory'
import type { ReviewItem } from '../../../types/memory'
import ContextDrawer from '../../layout/ContextDrawer.vue'
import BaseButton from '../../common/buttons/BaseButton.vue'
import LoadingState from '../../common/feedback/LoadingState.vue'
import ErrorState from '../../common/feedback/ErrorState.vue'
import EmptyState from '../../common/feedback/EmptyState.vue'

// The model's suggestions from "Review this chat for memories": the operator ticks what to keep. Presentational; the
// owner (useMemoryReview) holds the state and does the saving.
defineProps<{
  open: boolean
  loading: boolean
  saving: boolean
  error: string
  items: ReviewItem[]
  selectedCount: number
}>()

defineEmits<{
  (e: 'update:open', open: boolean): void
  (e: 'toggle', index: number): void
  (e: 'save'): void
  (e: 'retry'): void
}>()
</script>

<template>
  <ContextDrawer :open="open" title="Review for memories" wide @update:open="$emit('update:open', $event)">
    <div class="flex flex-col gap-4">
      <p class="m-0 text-[length:var(--text-small)] text-muted">
        The model read this conversation and suggests what to remember. Nothing is saved until you press Save selected.
      </p>

      <LoadingState v-if="loading" label="Reading this conversation" :rows="3" />
      <ErrorState
        v-else-if="error && items.length === 0"
        title="Could not review this conversation"
        :cause="error"
        next="Nothing was saved and the conversation is unchanged."
      >
        <template #action><BaseButton variant="secondary" icon="refresh" @click="$emit('retry')">Try again</BaseButton></template>
      </ErrorState>
      <EmptyState
        v-else-if="items.length === 0"
        title="No suggestions"
        body="Nothing in this conversation looks worth remembering. You can still add a memory by hand in the Memory panel."
      />

      <template v-else>
        <ul data-test="review-list" class="m-0 flex list-none flex-col p-0">
          <li v-for="(item, index) in items" :key="item.content" class="flex items-start gap-3 border-b border-hairline py-3 last:border-b-0">
            <input
              :id="`review-${index}`"
              type="checkbox"
              :checked="item.selected"
              :disabled="item.duplicate || saving"
              class="mt-1 h-5 w-5 flex-none cursor-pointer focus-visible:outline-none focus-visible:ring-2 disabled:cursor-not-allowed sm:h-4 sm:w-4"
              @change="$emit('toggle', index)"
            />
            <label :for="`review-${index}`" class="flex min-w-0 flex-1 cursor-pointer flex-col gap-1">
              <span class="break-words text-[length:var(--text-body)] text-primary">{{ item.content }}</span>
              <span class="flex flex-wrap items-center gap-x-2 font-mono text-[length:var(--text-micro)] text-faint">
                <span>{{ MEMORY_SCOPE_LABEL[item.scope] }}</span>
                <span aria-hidden="true">·</span>
                <span>{{ MEMORY_RECALL_LABEL[item.mode] }}</span>
                <template v-if="item.duplicate"><span aria-hidden="true">·</span><span>Already saved</span></template>
                <template v-if="item.failed"><span aria-hidden="true">·</span><span class="text-state-error">Could not be saved</span></template>
              </span>
            </label>
          </li>
        </ul>
        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <BaseButton variant="ghost" @click="$emit('update:open', false)">Cancel</BaseButton>
          <BaseButton :disabled="selectedCount === 0 || saving" @click="$emit('save')">
            {{ saving ? 'Saving…' : `Save selected (${selectedCount})` }}
          </BaseButton>
        </div>
      </template>
    </div>
  </ContextDrawer>
</template>
