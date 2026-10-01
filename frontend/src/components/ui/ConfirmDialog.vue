<script setup lang="ts">
import { nextTick, ref, useId, watch } from 'vue'
import type { DialogType } from '../../types/ui'
import BaseButton from '../common/buttons/BaseButton.vue'

// The single confirmation dialog (Phase 5 primitive), shown through useConfirm.
// An alert dialog: it takes focus on the safe choice (Cancel), Escape cancels,
// and focus returns to whatever opened it. Warning and error dialogs mark the
// confirm action as destructive.

interface Props {
  modelValue: boolean
  title: string
  message: string
  type?: DialogType
  confirmText?: string
  cancelText?: string
}

const props = withDefaults(defineProps<Props>(), {
  type: 'info',
  confirmText: 'Confirm',
  cancelText: 'Cancel'
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()

const titleId = useId()
const messageId = useId()
const cancelButton = ref<InstanceType<typeof BaseButton> | null>(null)
let opener: HTMLElement | null = null

const close = () => { emit('update:modelValue', false) }
const confirm = () => { emit('confirm'); close() }
const cancel = () => { emit('cancel'); close() }

watch(
  () => props.modelValue,
  async (open) => {
    if (open) {
      opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
      await nextTick()
      ;(cancelButton.value?.$el as HTMLElement | undefined)?.focus()
    } else {
      opener?.focus()
      opener = null
    }
  },
  { immediate: true },
)
</script>

<template>
  <div v-if="modelValue" class="fixed inset-0 z-50 flex items-center justify-center bg-scrim/60 p-4">
    <div
      role="alertdialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      :aria-describedby="messageId"
      :data-type="type"
      class="w-full max-w-md rounded-[var(--radius-sm)] border border-strong bg-surface p-5"
      @keydown.escape="cancel"
    >
      <h2 :id="titleId" class="m-0 text-[15px] font-semibold text-primary">{{ title }}</h2>
      <p :id="messageId" class="mb-5 mt-2 text-secondary">{{ message }}</p>
      <div class="flex flex-wrap justify-end gap-2">
        <BaseButton ref="cancelButton" variant="secondary" @click="cancel">{{ cancelText }}</BaseButton>
        <BaseButton :variant="type === 'info' ? 'primary' : 'danger'" @click="confirm">{{ confirmText }}</BaseButton>
      </div>
    </div>
  </div>
</template>
