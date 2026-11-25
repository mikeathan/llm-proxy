<script setup lang="ts">
import { ref, useId } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import PopoverPanel from "../common/layout/PopoverPanel.vue"
import { useDismissable } from "../../composables/ui/useDismissable"
import { BRIEF_DELETE_LABEL, BRIEF_DELETE_THIS_LABEL, briefDeleteOlderWords } from "../../constants/newsroom"

// One trash button beside the brief picker: it opens the two ways to delete written briefs, the one on screen or the older ones of the same brief. It only
// says which the owner picked; the page asks before anything is deleted and does the deleting.
// olderCount only says whether older briefs are listed: the list holds the newest few, so it is not the number the server would delete.
defineProps<{ olderCount: number; editionName: string }>()
const emit = defineEmits<{ (e: "delete", older: boolean): void }>()

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const panel = ref<HTMLElement | null>(null)
const panelId = useId()
useDismissable(root, open, panel)

function pick(older: boolean) {
  open.value = false
  emit("delete", older)
}
</script>

<template>
  <div ref="root" class="relative">
    <BaseButton
      size="sm"
      variant="ghost"
      icon="trash"
      icon-only
      :label="BRIEF_DELETE_LABEL"
      aria-haspopup="dialog"
      :aria-expanded="open"
      :aria-controls="open ? panelId : undefined"
      data-test="brief-delete"
      @click="open = !open"
    />
    <div v-if="open" ref="panel">
      <PopoverPanel :id="panelId" :title="BRIEF_DELETE_LABEL" width="sm:w-[300px]" @close="open = false">
        <div class="flex flex-col gap-1 p-2">
          <BaseButton size="sm" variant="ghost" class-name="justify-start" data-test="brief-delete-this" @click="pick(false)">{{ BRIEF_DELETE_THIS_LABEL }}</BaseButton>
          <BaseButton v-if="olderCount > 0" size="sm" variant="ghost" class-name="justify-start" data-test="brief-delete-older" @click="pick(true)">{{ briefDeleteOlderWords(editionName) }}</BaseButton>
        </div>
      </PopoverPanel>
    </div>
  </div>
</template>
