<script setup lang="ts">
import BaseButton from "../buttons/BaseButton.vue"

// The bar select mode shows above a list (files, automations): a Select all
// checkbox, the live count, and one Delete selected action. The caller owns the
// selection; this only renders it and reports what the user asked for.
// `inside` counts entries that ride along with a ticked folder.
defineProps<{
  count: number
  allSelected: boolean
  someSelected: boolean
  inside?: number
}>()

const emit = defineEmits<{
  (e: "toggle-all", on: boolean): void
  (e: "delete"): void
  (e: "done"): void
}>()
</script>

<template>
  <div class="flex flex-col gap-2 rounded-[var(--radius-md)] border border-hairline bg-surface-raised p-2">
    <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
      <label class="flex min-h-6 cursor-pointer items-center gap-2 text-[length:var(--text-small)] text-primary">
        <input
          type="checkbox"
          aria-label="Select all"
          :checked="allSelected"
          :indeterminate="someSelected"
          class="h-4 w-4 flex-none cursor-pointer focus-visible:outline-none focus-visible:ring-2 sm:h-3.5 sm:w-3.5"
          @change="emit('toggle-all', ($event.target as HTMLInputElement).checked)"
        />
        Select all
      </label>
      <span role="status" class="ml-auto font-mono text-[length:var(--text-small)] tabular-nums text-muted">{{ count }} selected<template v-if="inside"> (+{{ inside }} inside)</template></span>
    </div>
    <div class="flex items-center gap-1.5">
      <BaseButton variant="danger" size="sm" icon="trash" class-name="flex-1" :disabled="!count" @click="emit('delete')">Delete selected</BaseButton>
      <BaseButton variant="ghost" size="sm" @click="emit('done')">Done</BaseButton>
    </div>
  </div>
</template>
