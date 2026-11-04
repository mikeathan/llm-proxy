<script setup lang="ts">
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { useNetauditHelp } from "../../composables/netaudit/useNetauditHelp"
import { HELP_LABEL, VIEW_MODE_OPTIONS } from "../../constants/netaudit"
import type { NetauditViewMode } from "../../types/netaudit"

// Top right of every network scan screen: Simple or Advanced (remembered, shared by all the screens) and Help. It is one
// short row that never wraps, so on a phone it can sit beside the page title; Help is a "?" to save the width.
const { mode, set } = useNetauditViewMode()
const help = useNetauditHelp()
</script>

<template>
  <div class="flex flex-none flex-nowrap items-center justify-end gap-0.5">
    <SegmentedControl :model-value="mode" :options="VIEW_MODE_OPTIONS" label="How much detail" variant="compact" @update:model-value="set($event as NetauditViewMode)" />
    <BaseButton variant="ghost" size="sm" class-name="relative min-w-[26px] before:absolute before:-inset-x-2.5 before:-inset-y-2.5 before:content-['']" :aria-label="HELP_LABEL" :title="HELP_LABEL" @click="help.show()">?</BaseButton>
  </div>
</template>
