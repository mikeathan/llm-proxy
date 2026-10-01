<script setup lang="ts">
import BaseButton from "../common/buttons/BaseButton.vue"
import UnsavedTag from "../common/display/UnsavedTag.vue"

// The actions row every editable Settings section ends with: the unsaved
// marker, section-specific actions (slot), Discard and Save. Save and Discard
// are live only while there is something to save; a failed save stays visible
// here, next to the button that caused it.
defineProps<{ dirty: boolean; saving?: boolean; saveLabel: string; error?: string }>()
defineEmits<{ (e: "save"): void; (e: "discard"): void }>()
defineSlots<{ default?(): unknown }>()
</script>

<template>
  <div class="flex flex-col gap-2">
    <div class="flex flex-wrap items-center justify-end gap-2">
      <UnsavedTag v-if="dirty" class="mr-auto" />
      <slot />
      <BaseButton variant="ghost" :disabled="!dirty || saving" @click="$emit('discard')">Discard changes</BaseButton>
      <BaseButton variant="primary" :loading="saving" :disabled="!dirty" @click="$emit('save')">{{ saveLabel }}</BaseButton>
    </div>
    <p v-if="error" role="alert" class="m-0 text-right text-[length:var(--text-small)] text-state-error">{{ error }}</p>
  </div>
</template>
