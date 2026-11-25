<script setup lang="ts">
import { ref, useId } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import PopoverPanel from "../common/layout/PopoverPanel.vue"
import { useDismissable } from "../../composables/ui/useDismissable"
import { BRIEF_DELETE_THIS_LABEL, BRIEF_MORE_LABEL, MAKE_ANOTHER_LABEL, briefDeleteOlderWords } from "../../constants/newsroom"

// One ⋯ button in the brief's masthead row for its rare actions: write a new one now (the scheduled brief comes on its own), delete the brief on
// screen, or delete the older ones of the same brief. It only says which the owner picked; the page asks before anything is deleted, and shows a
// write's progress itself, so the item is disabled while one runs.
// olderCount only says whether older briefs are listed: the list holds the newest few, so it is not the number the server would delete.
defineProps<{ olderCount: number; editionName: string; writing: boolean }>()
const emit = defineEmits<{ (e: "delete", older: boolean): void; (e: "write"): void }>()

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const panel = ref<HTMLElement | null>(null)
const panelId = useId()
useDismissable(root, open, panel)

function pickDelete(older: boolean) {
  open.value = false
  emit("delete", older)
}
function pickWrite() {
  open.value = false
  emit("write")
}
</script>

<template>
  <div ref="root" class="relative">
    <BaseButton
      variant="ghost"
      icon="more"
      icon-only
      :label="BRIEF_MORE_LABEL"
      class-name="max-sm:!h-11 max-sm:!w-11"
      aria-haspopup="dialog"
      :aria-expanded="open"
      :aria-controls="open ? panelId : undefined"
      data-test="brief-more"
      @click="open = !open"
    />
    <div v-if="open" ref="panel">
      <PopoverPanel :id="panelId" :title="BRIEF_MORE_LABEL" width="sm:w-[300px]" @close="open = false">
        <div class="flex flex-col gap-1 p-2">
          <BaseButton size="sm" variant="ghost" :disabled="writing" class-name="justify-start max-sm:!h-11" data-test="write-now" @click="pickWrite">{{ MAKE_ANOTHER_LABEL }}</BaseButton>
          <div class="mt-1 flex flex-col gap-1 border-t border-hairline pt-1">
            <BaseButton size="sm" variant="ghost" class-name="justify-start !text-state-error max-sm:!h-11" data-test="brief-delete-this" @click="pickDelete(false)">{{ BRIEF_DELETE_THIS_LABEL }}</BaseButton>
            <BaseButton v-if="olderCount > 0" size="sm" variant="ghost" class-name="justify-start !text-state-error max-sm:!h-11" data-test="brief-delete-older" @click="pickDelete(true)">{{ briefDeleteOlderWords(editionName) }}</BaseButton>
          </div>
        </div>
      </PopoverPanel>
    </div>
  </div>
</template>
