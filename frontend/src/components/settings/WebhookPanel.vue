<script setup lang="ts">
import { inject, computed } from "vue"
import type { Ref } from "vue"
import type { ConnectorConfig, VerifyState } from "../../types/admin"
import type { StatusState } from "../../types/ui"
import { useWebhook } from "../../composables/useWebhook"
import { useConfirm } from "../../composables/ui/useConfirm"
import BaseButton from "../common/buttons/BaseButton.vue"
import CopyButton from "../common/display/CopyButton.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import StatusTag from "../common/display/StatusTag.vue"
import FormField from "../common/forms/FormField.vue"
import Panel from "../common/layout/Panel.vue"

const props = defineProps<{
  name: string
  cfg: ConnectorConfig
}>()

// Parent provides these as writable Refs so the composable can mutate connectors
// (via the computed setter) and surface errors through the shared saveError display.
const connectors = inject("connectors") as unknown as Ref<Record<string, ConnectorConfig>>
const saveError = inject("saveError") as unknown as Ref<string>
const { webhookBaseUrl, defaultHost, webhookState, createWebhook, verifyWebhook, deleteWebhook } = useWebhook(connectors, saveError)
const { confirm } = useConfirm()

// Computed alias so the template reads s.host/s.creating instead of webhookState(name).host/...
const s = computed(() => webhookState(props.name))
const localEndpoint = computed(() => `${webhookBaseUrl.value}${props.name}`)

const VERIFY_TAG: Record<Exclude<VerifyState, "idle">, { state: StatusState; label: string }> = {
  checking: { state: "running", label: "Checking" },
  registered: { state: "success", label: "Registered" },
  unregistered: { state: "neutral", label: "Not registered" },
  error: { state: "error", label: "Error" },
}
const verifyTag = computed(() => (s.value.verifyState === "idle" ? null : VERIFY_TAG[s.value.verifyState]))
const urlMismatch = computed(
  () => s.value.verifyState === "registered" && !!s.value.info?.url && !!props.cfg.webhook_url && s.value.info.url !== props.cfg.webhook_url,
)

async function remove() {
  const ok = await confirm({
    title: `Delete the webhook of ${props.name}?`,
    message: "Telegram stops delivering inbound messages to this connector until a webhook is created again.",
    type: "warning",
    confirmText: "Delete webhook",
  })
  if (ok) await deleteWebhook(props.name)
}
</script>

<template>
  <Panel :title="`Inbound webhook · ${name}`" preserve-case>
    <template #actions>
      <StatusTag v-if="verifyTag" v-bind="verifyTag" />
    </template>
    <dl class="m-0 grid grid-cols-[max-content_minmax(0,1fr)] items-center gap-x-4 gap-y-2">
      <dt><MicroLabel>Registered</MicroLabel></dt>
      <dd class="m-0 flex min-w-0 items-center gap-2">
        <code v-if="cfg.webhook_url" class="truncate font-mono text-[length:var(--text-small)] text-primary">{{ cfg.webhook_url }}</code>
        <span v-else class="text-[length:var(--text-small)] text-faint">Not registered yet</span>
        <CopyButton v-if="cfg.webhook_url" :text="cfg.webhook_url" title="Copy registered URL" />
      </dd>
      <dt><MicroLabel>Local endpoint</MicroLabel></dt>
      <dd class="m-0 flex min-w-0 items-center gap-2">
        <code class="truncate font-mono text-[length:var(--text-small)] text-muted">{{ localEndpoint }}</code>
        <CopyButton :text="localEndpoint" title="Copy local endpoint" />
      </dd>
    </dl>

    <div class="mt-4 flex flex-wrap items-end gap-2">
      <FormField label="Public host" hint="A tunnel host or full URL that reaches this server." class="min-w-[220px] max-w-sm flex-1">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="s.host" :aria-describedby="describedBy" :placeholder="defaultHost" type="text" class="form-control font-mono" autocomplete="off" />
        </template>
      </FormField>
      <BaseButton variant="secondary" size="sm" icon="play" :loading="s.creating" @click="createWebhook(name)">Create</BaseButton>
      <BaseButton variant="secondary" size="sm" icon="check" :loading="s.verifying" @click="verifyWebhook(name)">Verify</BaseButton>
      <BaseButton variant="danger" size="sm" icon="trash" :loading="s.deleting" @click="remove">Delete</BaseButton>
    </div>

    <p v-if="s.verifyMsg" class="mb-0 mt-3 text-[length:var(--text-small)] text-muted">{{ s.verifyMsg }}</p>
    <p v-if="urlMismatch" class="mb-0 mt-1 text-[length:var(--text-small)] text-state-running">
      Telegram sees a different URL — create the webhook again with the current host.
    </p>
    <p v-if="s.statusMsg" role="status" class="mb-0 mt-1 text-[length:var(--text-small)] text-state-success">{{ s.statusMsg }}</p>
  </Panel>
</template>
