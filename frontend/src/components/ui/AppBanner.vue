<script setup lang="ts">
import { computed, inject } from 'vue'
import { useAppBanner } from '../../composables/ui/useAppBanner'
import Icon from '../icons/Icon.vue'

const { active, clear } = useAppBanner()

const setActiveSettingsTab = inject<(tab: string) => void>('setActiveSettingsTab')

const message = computed(() => active.value)
const dismissable = computed(() => (message.value ? !message.value.persistent : false))

const content = computed(() => message.value?.html ?? message.value?.message ?? '')
// Only treat as HTML when an explicit `html` payload was provided; otherwise the
// text fallback is rendered as plain text to avoid accidental injection.
const isHtml = computed(() => !!message.value?.html)

function onAction() {
  const action = message.value?.action
  if (action && setActiveSettingsTab) {
    setActiveSettingsTab(action.settingsTab)
  }
}
</script>

<template>
  <div
    v-if="message"
    class="banner"
    :class="{
      'banner-critical': message.severity === 'critical',
      'banner-notice': message.severity === 'notice',
      'banner-error': message.severity === 'error',
    }"
  >
    <div class="banner-content">
      <div class="banner-message-row">
        <Icon name="warning" size="sm" className="shrink-0 mt-0.5" />
        <span class="banner-text">
          <span v-if="isHtml" v-html="content"></span>
          <template v-else>{{ content }}</template>
        </span>
        <button
          v-if="message.action"
          type="button"
          class="banner-action"
          @click="onAction"
        >
          {{ message.action.label }}
        </button>
      </div>
      <button
        v-if="dismissable"
        @click="clear"
        class="btn-dismiss"
        title="Dismiss"
      >
        <Icon name="close" size="sm" />
      </button>
    </div>
  </div>
</template>

<style scoped lang="postcss">
/* In-flow banner: occupies layout space below the sticky header so it never
   overlays or blocks the top navigation buttons. */
.banner {
  @apply relative w-full z-20 p-3 border-b
         flex flex-col gap-2 animate-in slide-in-from-top duration-300;
}

.banner-content {
  @apply max-w-7xl mx-auto w-full flex items-center justify-between gap-3;
}

.banner-message-row {
  @apply flex items-center gap-2 flex-wrap;
}

.banner-critical {
  @apply bg-amber-900/40 border-amber-700/50;

  & .banner-message-row {
    @apply text-amber-200;
  }
  & .btn-dismiss {
    @apply text-amber-400 hover:text-amber-100;
  }
  & .banner-action {
    @apply bg-amber-700/60 border-amber-500/60 text-amber-50 hover:bg-amber-600/70;
  }
}

.banner-notice {
  @apply bg-blue-900/40 border-blue-700/50;

  & .banner-message-row {
    @apply text-blue-200;
  }
  & .btn-dismiss {
    @apply text-blue-400 hover:text-blue-100;
  }
  & .banner-action {
    @apply bg-blue-700/60 border-blue-500/60 text-blue-50 hover:bg-blue-600/70;
  }
}

.banner-error {
  @apply bg-red-900/40 border-red-700/50;

  & .banner-message-row {
    @apply text-red-200;
  }
  & .btn-dismiss {
    @apply text-red-400 hover:text-red-100;
  }
  & .banner-action {
    @apply bg-red-700/60 border-red-500/60 text-red-50 hover:bg-red-600/70;
  }
}

.banner-text {
  @apply text-xs leading-snug font-medium;
}

.banner-action {
  @apply ml-2 shrink-0 px-3 py-1 rounded-md text-xs font-semibold
         border transition-colors active:scale-95;
}

.btn-dismiss {
  @apply shrink-0 p-1 -m-1 transition-colors rounded-full hover:bg-white/10;
}
</style>
