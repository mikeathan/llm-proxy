<script setup lang="ts">
import { ref, watch } from "vue"
import { useRoute } from "vue-router"
import BrandMark from "../components/layout/BrandMark.vue"
import { toOverview, toWorkspaces } from "../router/routes"
import { ROUTE_NAMES } from "../types/routes"

// Rendered inside the shell for any unknown path (plan D18), so a bad link is
// visible and recoverable — never a silent redirect. The path follows only
// not-found routes, so it does not flash the next page's path while leaving.
const route = useRoute()
const missingPath = ref(route.fullPath)
watch(() => route.fullPath, (path) => {
  if (route.name === ROUTE_NAMES.notFound) missingPath.value = path
})
</script>

<template>
  <section class="flex flex-col items-start gap-3 rounded-[var(--radius-sm)] border border-hairline bg-surface p-6" aria-labelledby="not-found-title">
    <BrandMark :size="20" muted />
    <p class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">404 · No route</p>
    <h1 id="not-found-title" class="text-[length:var(--text-display)] font-semibold leading-tight text-primary">
      No page at <span class="break-all font-mono">{{ missingPath }}</span>
    </h1>
    <p class="text-secondary">The link may be from an older version, or the workspace was renamed.</p>
    <div class="mt-2 flex flex-wrap gap-2">
      <RouterLink
        :to="toOverview()"
        class="rounded-[var(--radius-md)] border border-control bg-text-primary px-3 py-1.5 font-mono text-[length:var(--text-small)] text-inverse offset-brand focus-visible:outline-none focus-visible:ring-2"
      >Go to Overview</RouterLink>
      <RouterLink
        :to="toWorkspaces()"
        class="rounded-[var(--radius-md)] border border-control px-3 py-1.5 font-mono text-[length:var(--text-small)] text-primary hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2"
      >All workspaces</RouterLink>
    </div>
  </section>
</template>
