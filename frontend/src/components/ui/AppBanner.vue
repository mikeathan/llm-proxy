<script setup lang="ts">
import { computed } from 'vue'
import { useAppBanner } from '../../composables/ui/useAppBanner'
import type { BannerSeverity } from '../../types/ui'
import Icon from '../icons/Icon.vue'
import BaseButton from '../common/buttons/BaseButton.vue'

// The app-wide banner (one message at a time, from useAppBanner). An error is
// an alert; a notice or a standing configuration warning is a polite status.
const { active, clear } = useAppBanner()

const message = computed(() => active.value)
const dismissable = computed(() => (message.value ? !message.value.persistent : false))

const content = computed(() => message.value?.html ?? message.value?.message ?? '')
// Only treat as HTML when an explicit `html` payload was provided; otherwise the
// text fallback is rendered as plain text to avoid accidental injection.
const isHtml = computed(() => !!message.value?.html)

const TONE: Record<BannerSeverity, string> = {
  critical: 'border-state-running bg-state-running/[0.08] text-state-running',
  notice: 'border-accent-info bg-accent-info/[0.08] text-accent-info-text',
  error: 'border-state-error bg-state-error/[0.08] text-state-error',
}
const role = computed(() => (message.value?.severity === 'error' ? 'alert' : 'status'))
</script>

<template>
  <!-- In-flow banner: occupies layout space below the sticky header so it never
       overlays or blocks the top navigation buttons. -->
  <div v-if="message" :role="role" :class="['relative z-20 w-full border-b px-3 py-2.5', TONE[message.severity]]">
    <div class="mx-auto flex w-full max-w-7xl items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2">
        <Icon name="warning" size="sm" className="shrink-0" />
        <span class="text-[length:var(--text-small)] font-medium leading-snug">
          <span v-if="isHtml" v-html="content"></span>
          <template v-else>{{ content }}</template>
        </span>
        <RouterLink
          v-if="message.action"
          :to="message.action.to"
          class="ml-2 inline-flex h-[26px] flex-none items-center rounded-[var(--radius-sm)] border border-current px-2.5 font-mono text-[length:var(--text-small)] font-medium hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2"
        >
          {{ message.action.label }}
        </RouterLink>
      </div>
      <BaseButton v-if="dismissable" variant="ghost" size="sm" icon="close" icon-only label="Dismiss" @click="clear" />
    </div>
  </div>
</template>
