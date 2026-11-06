<script setup lang="ts">
import { computed } from "vue"
import { useModuleSources } from "../../composables/netaudit/useModuleSources"
import { toNetwork } from "../../router/routes"
import { FOUND_BY_OFF, FOUND_BY_PREFIX } from "../../constants/stacks"

// One quiet line under a kind of problem: which module found it, and whether that module is on. It shows nothing when no single
// module produces the check (the audit's own scan does), or before the modules have been read.
const props = defineProps<{ check: string }>()
const { sourceOf } = useModuleSources()
const source = computed(() => sourceOf(props.check))
</script>

<template>
  <p v-if="source" class="m-0 text-[length:var(--text-small)] text-muted" data-test="found-by">
    {{ FOUND_BY_PREFIX }}
    <RouterLink :to="toNetwork('modules')" class="text-accent-info-text underline-offset-2 hover:underline">{{ source.title }}</RouterLink>
    <template v-if="!source.enabled"> &middot; {{ FOUND_BY_OFF }}</template>
  </p>
</template>
