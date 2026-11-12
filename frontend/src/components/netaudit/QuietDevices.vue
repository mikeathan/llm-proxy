<script setup lang="ts">
import Disclosure from "../common/layout/Disclosure.vue"
import { computed, ref } from "vue"
import { deviceLabel } from "../../utils/netaudit/friendly"

// The devices that answered the scan and had nothing open: they are in no other list, so without this a report says "15 devices up"
// and shows ten. One folded line; each device with the name the person gave it. Presentation only.
const props = defineProps<{ devices: readonly string[]; names?: Readonly<Record<string, string>> }>()
const open = ref(false)
const summary = computed(() => `${props.devices.length} ${props.devices.length === 1 ? "device" : "devices"}`)
</script>

<template>
  <Disclosure v-if="devices.length" title="Answered, nothing open" :summary="summary" :open="open" data-test="quiet-devices" @update:open="open = $event">
    <ul class="m-0 flex list-none flex-col p-0">
      <li v-for="d in devices" :key="d" data-test="quiet-device" class="flex flex-wrap items-baseline gap-x-3 border-t border-hairline px-4 py-2 first:border-t-0">
        <span class="break-all font-mono text-primary">{{ deviceLabel(d) }}</span>
        <span v-if="names?.[d]" class="min-w-0 break-words text-[length:var(--text-small)] text-muted">{{ names[d] }}</span>
      </li>
    </ul>
  </Disclosure>
</template>
