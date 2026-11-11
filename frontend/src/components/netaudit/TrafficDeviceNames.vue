<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { flagWords, groupLookups } from "../../utils/netaudit/lookups"
import { formatRelativeTime } from "../../utils/format/time"
import { toNetworkSettings } from "../../router/routes"
import {
  LOOKUPS_COUNT, LOOKUPS_FEWER, LOOKUPS_FLAG_HELP, LOOKUPS_LAST, LOOKUPS_NO_NAMES, LOOKUPS_OFF, LOOKUPS_OPEN_SETTINGS, LOOKUPS_SHOW_ALL, NAMES_NOTE, NAMES_SHOWN, NAMES_TITLE, NAMES_UNKNOWN,
} from "../../constants/netaudit"
import type { DeviceLookups } from "../../types/netaudit"

// What this device asked the router's DNS for (Constitution I.8), beside where it connected: the names, grouped by domain, newest first, with the
// flags the watch raised. Lookups, not traffic; the names are text a device chose, shown as text only and never sent anywhere.
const props = defineProps<{ lookups: DeviceLookups | null; enabled: boolean; known: boolean; error?: string }>()
const showAll = ref(false)
const groups = computed(() => groupLookups(props.lookups?.names ?? []))
const shown = computed(() => (showAll.value ? groups.value : groups.value.slice(0, NAMES_SHOWN)))
const flags = computed(() => flagWords(props.lookups?.flags ?? []))
</script>

<template>
  <section class="flex flex-col gap-2" data-test="device-names">
    <h3 class="m-0 font-mono text-[length:var(--text-micro)] font-normal uppercase tracking-[var(--tracking-micro)] text-muted">{{ NAMES_TITLE }}</h3>
    <p v-if="error" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="names-error">{{ error }}</p>
    <div v-if="!enabled" class="flex flex-wrap items-center gap-3" data-test="names-off">
      <p class="m-0 text-muted">{{ LOOKUPS_OFF }}</p>
      <RouterLink :to="toNetworkSettings('outside')" class="inline-flex min-h-11 items-center text-accent-info-text underline-offset-2 hover:underline sm:min-h-8">{{ LOOKUPS_OPEN_SETTINGS }}</RouterLink>
    </div>
    <p v-else-if="!known" class="m-0 text-muted" data-test="names-unknown">{{ NAMES_UNKNOWN }}</p>
    <template v-else-if="lookups">
      <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="names-counts">{{ LOOKUPS_COUNT(lookups.today, lookups.total) }}<template v-if="lookups.last"> · {{ LOOKUPS_LAST(formatRelativeTime(lookups.last)) }}</template></p>
      <p v-if="flags.length" class="m-0 flex flex-wrap gap-2" data-test="names-flags">
        <span v-for="f in flags" :key="f.flag" :title="LOOKUPS_FLAG_HELP[f.flag]" class="rounded-full border border-state-running px-2 py-0.5 text-[length:var(--text-small)] text-state-running">{{ f.label }}</span>
      </p>
      <p v-if="!groups.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="names-none">{{ LOOKUPS_NO_NAMES }}</p>
      <ul v-else class="m-0 flex list-none flex-col gap-2 p-0" data-test="names-list">
        <li v-for="g in shown" :key="g.domain" class="flex flex-col" data-test="names-group">
          <span class="flex flex-wrap items-baseline gap-x-2">
            <span class="break-all font-mono text-primary">{{ g.domain }}</span>
            <span v-if="g.flag" class="text-[length:var(--text-small)] text-state-running">{{ flagWords([g.flag])[0]!.label }}</span>
            <span class="text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(g.last) }}</span>
          </span>
          <span v-if="g.names.length > 1 || g.names[0]!.name !== g.domain" class="break-all text-[length:var(--text-small)] text-muted">{{ g.names.map((n) => n.name).join(", ") }}</span>
        </li>
      </ul>
      <div v-if="groups.length > NAMES_SHOWN">
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="names-more" @click="showAll = !showAll">{{ showAll ? LOOKUPS_FEWER : LOOKUPS_SHOW_ALL(groups.length - NAMES_SHOWN) }}</BaseButton>
      </div>
    </template>
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ NAMES_NOTE }}</p>
  </section>
</template>
