<script setup lang="ts">
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { useNetauditHelp } from "../../composables/netaudit/useNetauditHelp"
import { VIEW_MODE_OPTIONS } from "../../constants/netaudit"
import type { NetauditViewMode } from "../../types/netaudit"

// Top right of every network scan screen: Simple or Advanced (remembered, shared by all the screens) and Help.
const { mode, set } = useNetauditViewMode()
const help = useNetauditHelp()
</script>

<template>
  <div class="flex flex-wrap items-center justify-end gap-2">
    <SegmentedControl :model-value="mode" :options="VIEW_MODE_OPTIONS" label="How much detail" @update:model-value="set($event as NetauditViewMode)" />
    <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="help.show()">Help</BaseButton>
  </div>
</template>
