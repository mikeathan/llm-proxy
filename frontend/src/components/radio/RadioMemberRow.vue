<script setup lang="ts">
import { computed } from "vue"
import Icon from "../icons/Icon.vue"
import { radioDbmLine } from "../../constants/radio"
import { RADIO_KIND_ICON, RADIO_KIND_WORDS, RADIO_OWNERSHIP_WORDS } from "../../constants/radioIdentity"
import { toRadioDevice } from "../../router/routes"
import { conclusionOf, nameOf, ownershipOf } from "../../utils/radio/identity"
import type { RadioMember } from "../../types/radio"

// Radio · a device as a line that opens its page: its icon, its name, whose it probably is, what it is and how strong. Used by the networks view,
// the device page (its router's radios, its clients) and the "help it learn" card.
const props = defineProps<{ member: RadioMember }>()
const own = computed(() => ownershipOf(props.member))
const kind = computed(() => props.member.ident?.kind ?? "unknown")
</script>

<template>
  <RouterLink :to="toRadioDevice(member.id)" class="flex min-h-11 items-center gap-3 py-1.5 text-left hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2" :class="member.heard ? '' : 'opacity-60'" data-test="radio-member" :data-id="member.id" :data-own="own">
    <span class="flex h-7 w-7 flex-none items-center justify-center rounded-[var(--radius-md)] border" :class="own === 'mine' || own === 'likely' ? 'border-accent-brand text-accent-brand' : 'border-hairline text-muted'" aria-hidden="true"><Icon :name="RADIO_KIND_ICON[kind]" size="sm" /></span>
    <span class="flex min-w-0 flex-1 flex-col">
      <span class="min-w-0 truncate text-primary" data-test="radio-member-name">{{ nameOf(member) }}</span>
      <span class="min-w-0 truncate text-[length:var(--text-small)] text-muted">{{ own === 'mine' ? RADIO_OWNERSHIP_WORDS.mine : conclusionOf(member) }}<template v-if="kind !== 'unknown'"> · {{ RADIO_KIND_WORDS[kind] }}</template></span>
    </span>
    <span class="flex-none font-mono text-[length:var(--text-small)] text-muted">{{ radioDbmLine(member.dbm) }}</span>
    <Icon name="chevron-right" size="sm" class-name="flex-none text-faint" />
  </RouterLink>
</template>
