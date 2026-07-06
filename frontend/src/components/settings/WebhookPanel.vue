<script setup lang="ts">
import { inject, computed } from "vue"
import type { Ref } from "vue"
import type { ConnectorConfig } from "../../types/admin"
import { useWebhook } from "../../composables/useWebhook"
import BaseButton from "../common/buttons/BaseButton.vue"
import CopyButton from "../common/display/CopyButton.vue"

const props = defineProps<{
  name: string
  cfg: ConnectorConfig
}>()

// Parent provides these as writable Refs so the composable can mutate connectors
// (via the computed setter) and surface errors through the shared saveError display.
const connectors = inject("connectors") as unknown as Ref<Record<string, ConnectorConfig>>
const saveError = inject("saveError") as unknown as Ref<string>
const { webhookBaseUrl, defaultHost, webhookState, createWebhook, verifyWebhook, deleteWebhook } = useWebhook(connectors, saveError)

// Computed alias so the template reads s.host/s.creating instead of webhookState(name).host/...
const s = computed(() => webhookState(props.name))
</script>

<template>
  <div class="webhook-panel">
    <div class="webhook-url-row">
      <span class="webhook-label">Registered:</span>
      <code v-if="cfg.webhook_url" class="webhook-url">{{ cfg.webhook_url }}</code>
      <span v-else class="webhook-url webhook-url--empty">Not registered yet</span>
      <CopyButton v-if="cfg.webhook_url" :text="cfg.webhook_url" iconSize="sm" />
    </div>
    <div class="webhook-url-row webhook-url-row--hint">
      <span class="webhook-label">Local endpoint:</span>
      <code class="webhook-url webhook-url--hint">{{ webhookBaseUrl }}{{ name }}</code>
      <CopyButton :text="`${webhookBaseUrl}${name}`" iconSize="sm" />
    </div>

    <div class="webhook-controls">
      <input v-model="s.host"
             :placeholder="defaultHost"
             class="form-input form-input--inline form-input--host" />
      <BaseButton variant="primary" size="sm" icon="play"
                  :loading="s.creating" @click="createWebhook(name)">
        Create
      </BaseButton>
      <BaseButton variant="secondary" size="sm" icon="check"
                  :loading="s.verifying" @click="verifyWebhook(name)">
        Verify
      </BaseButton>
      <BaseButton variant="danger" size="sm" icon="trash"
                  :loading="s.deleting" @click="deleteWebhook(name)">
        Delete
      </BaseButton>
    </div>

    <div v-if="s.verifyState && s.verifyState !== 'idle'" class="verify-status">
      <span :class="['verify-badge', `verify-badge--${s.verifyState}`]">
        <template v-if="s.verifyState === 'checking'">Checking...</template>
        <template v-else-if="s.verifyState === 'registered'">Registered</template>
        <template v-else-if="s.verifyState === 'unregistered'">Not registered</template>
        <template v-else-if="s.verifyState === 'error'">Error</template>
      </span>
      <span v-if="s.verifyMsg" class="verify-msg">{{ s.verifyMsg }}</span>
      <span v-if="s.verifyState === 'registered' && s.info?.url && cfg.webhook_url && s.info!.url !== cfg.webhook_url" class="verify-mismatch">
        Telegram sees a different URL — re-create webhook with current host
      </span>
    </div>
    <div v-if="s.statusMsg" class="webhook-status">{{ s.statusMsg }}</div>
  </div>
</template>

<style scoped lang="postcss">
.webhook-panel {
  @apply space-y-2 py-2.5 px-3 bg-gray-900/60 rounded border border-gray-700/50;
}
.webhook-url-row {
  @apply flex items-center gap-2 text-xs;
}
.webhook-label {
  @apply text-gray-400 shrink-0;
}
.webhook-url {
  @apply text-blue-300 font-mono truncate flex-1;
}
.webhook-controls {
  @apply flex items-center gap-2;
}
.form-input--inline {
  @apply w-auto min-w-[180px] flex-1 max-w-sm;
}
.form-input--host {
  @apply font-mono text-xs;
}
.webhook-url-row--hint {
  @apply opacity-60;
}
.webhook-url--empty {
  @apply text-gray-500 italic;
}
.webhook-url--hint {
  @apply text-gray-500;
}
.verify-status {
  @apply flex flex-wrap items-center gap-x-3 gap-y-1 text-xs;
}
.verify-badge {
  @apply inline-flex items-center gap-1 font-medium;
}
.verify-badge--checking {
  @apply text-gray-400;
}
.verify-badge--registered {
  @apply text-green-400;
}
.verify-badge--unregistered {
  @apply text-amber-400;
}
.verify-badge--error {
  @apply text-red-400;
}
.verify-msg {
  @apply text-gray-400;
}
.verify-mismatch {
  @apply text-amber-400;
}
.webhook-status {
  @apply text-xs text-green-400;
}
</style>
