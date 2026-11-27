<script setup lang="ts">
import { toModels, toModelsProvider } from "../../router/routes";
import type { ProviderRailEntry, RailId } from "../../types/model";
import type { StatusState } from "../../types/ui";

// Models › every provider at once: All models, This computer, then each cloud provider with what it holds (keys and models, or "Not set up").
// Each entry is a link, so the back button and a shared address work. On a desktop it sits beside the pane; on a phone it is the list screen
// that the Providers row opens, and every entry is a 44px target.

defineProps<{ entries: readonly ProviderRailEntry[]; selected: RailId }>();

const DOT: Partial<Record<StatusState, string>> = { success: "bg-state-success", info: "bg-accent-info", error: "bg-state-error" };
const dotClass = (state: StatusState) => DOT[state] ?? "bg-faint";
const to = (id: RailId) => (id === "all" ? toModels() : toModelsProvider(id));
</script>

<template>
  <nav aria-label="Providers" class="border border-hairline bg-surface">
    <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
      <li v-for="e in entries" :key="e.id">
        <RouterLink
          :to="to(e.id)"
          :aria-current="selected === e.id ? 'page' : undefined"
          :data-provider="e.id"
          class="flex min-h-11 items-center gap-3 px-3 py-2 text-muted no-underline hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 aria-[current=page]:bg-surface-active aria-[current=page]:text-primary aria-[current=page]:shadow-[inset_2px_0_0_rgb(var(--accent-brand))]"
        >
          <span :class="['size-2 flex-none rounded-full', dotClass(e.state)]" aria-hidden="true"></span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[length:var(--text-body)] text-primary">{{ e.label }}</span>
            <span class="block truncate font-mono text-[length:var(--text-micro)] text-muted">{{ e.summary }}</span>
          </span>
          <span class="flex-none text-faint sm:hidden" aria-hidden="true">›</span>
        </RouterLink>
      </li>
    </ul>
  </nav>
</template>
