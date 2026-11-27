<script setup lang="ts">
import { ref, useId } from "vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import PopoverPanel from "../common/layout/PopoverPanel.vue";
import { useDismissable } from "../../composables/ui/useDismissable";

// One ⋯ button in a provider's header for its rare, destructive actions: remove every model of the provider, or every key. It only says which
// the owner picked; the page asks before anything is removed.

const props = defineProps<{ label: string; hasModels: boolean; hasKeys: boolean }>();
const emit = defineEmits<{ (e: "remove-models"): void; (e: "remove-keys"): void }>();

const open = ref(false);
const root = ref<HTMLElement | null>(null);
const panel = ref<HTMLElement | null>(null);
const panelId = useId();
useDismissable(root, open, panel);

function pick(action: "remove-models" | "remove-keys") {
  open.value = false;
  if (action === "remove-models") emit("remove-models");
  else emit("remove-keys");
}
</script>

<template>
  <div v-if="hasModels || hasKeys" ref="root" class="relative">
    <BaseButton
      variant="ghost"
      size="sm"
      icon="more"
      icon-only
      :label="`More actions for ${props.label}`"
      class-name="max-sm:!h-11 max-sm:!w-11"
      aria-haspopup="dialog"
      :aria-expanded="open"
      :aria-controls="open ? panelId : undefined"
      @click="open = !open"
    />
    <div v-if="open" ref="panel">
      <PopoverPanel :id="panelId" :title="props.label" width="sm:w-[280px]" @close="open = false">
        <div class="flex flex-col gap-1 p-2">
          <BaseButton v-if="hasModels" size="sm" variant="ghost" class-name="justify-start !text-state-error max-sm:!h-11" @click="pick('remove-models')">Remove all {{ props.label }} models</BaseButton>
          <BaseButton v-if="hasKeys" size="sm" variant="ghost" class-name="justify-start !text-state-error max-sm:!h-11" @click="pick('remove-keys')">Remove all keys</BaseButton>
        </div>
      </PopoverPanel>
    </div>
  </div>
</template>
