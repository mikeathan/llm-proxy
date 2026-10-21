<script setup lang="ts">
import { toWorkspaceFile } from "../../../router/routes";

// A link from a run to the file its final report was saved in (reports/<automation>/...), opened on the
// workspace's Files page. The caller shows it only for a run that kept a report.

defineProps<{
  workspaceId: string;
  path: string;
}>();
</script>

<template>
  <RouterLink
    :to="toWorkspaceFile(workspaceId, path)"
    aria-label="Open saved report"
    class="break-all font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2"
  >{{ path }}</RouterLink>
</template>
