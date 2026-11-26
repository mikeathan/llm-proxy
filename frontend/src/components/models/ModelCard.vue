<script setup lang="ts">
import { computed } from "vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import StatusTag from "../common/display/StatusTag.vue";
import { isLocalModel, modelStatus } from "../../domain/models";
import { formatTokenCount } from "../../utils/format/units";
import { formatElapsedSeconds, formatRelativeTime, isRecorded } from "../../utils/format/time";
import type { ActiveModel, Model } from "../../types/model";

// One model as a card on the Models page: its state, what it is, and for the running model how long it has run and when it last
// answered. Start / Stop for a local model (one runs at a time), Edit opens the side panel.

const props = defineProps<{
  model: Model;
  providerLabel: string;
  role: string | null;
  /** The loaded local model, when it is this one. */
  active?: ActiveModel;
  /** Whether Start is possible (no other local model is loaded). */
  canStart: boolean;
}>();
const emit = defineEmits<{ (e: "start"): void; (e: "stop"): void; (e: "edit"): void }>();

const MS_PER_SECOND = 1000;
const status = computed(() => modelStatus(props.model));
const local = computed(() => isLocalModel(props.model));
const meta = computed(() => {
  const m = props.model;
  if (!local.value) return [props.providerLabel, m.provider_config?.api_key_name ? `key ${m.provider_config.api_key_name}` : ""].filter(Boolean);
  return [
    m.metadata?.quantization ?? "",
    m.metadata?.context_length ? `${formatTokenCount(m.metadata.context_length)} context` : "",
    m.port ? `:${m.port}` : "",
  ].filter(Boolean);
});
const live = computed(() => {
  const a = props.active;
  if (!a) return "";
  const parts = [a.ready ? "ready" : "loading"];
  if (isRecorded(a.started_at)) parts.push(`up ${formatElapsedSeconds((Date.now() - Date.parse(a.started_at)) / MS_PER_SECOND)}`);
  if (isRecorded(a.last_used_at)) parts.push(`last call ${formatRelativeTime(a.last_used_at)}`);
  return parts.join(" · ");
});
</script>

<template>
  <article
    :class="['flex min-w-0 flex-col gap-2 rounded-[var(--radius-md)] border bg-surface p-3', model.active ? 'border-state-success' : 'border-hairline']"
    :data-model="model.name"
  >
    <div class="flex min-w-0 items-start gap-2">
      <h3 class="m-0 min-w-0 flex-1 break-words text-[length:var(--text-body)] font-semibold text-primary">{{ model.name }}</h3>
      <StatusTag v-if="model.active || !local" v-bind="status" />
      <StatusTag v-if="role" state="neutral" :label="role" />
    </div>
    <p class="m-0 font-mono text-[length:var(--text-small)] text-muted">{{ meta.join(" · ") }}</p>
    <p v-if="live" class="m-0 font-mono text-[length:var(--text-small)] text-secondary" data-test="live">{{ live }}</p>
    <div class="mt-auto flex flex-wrap gap-2 pt-1">
      <template v-if="local">
        <BaseButton v-if="model.active" variant="secondary" size="sm" icon="stop" @click="emit('stop')">Stop<span class="sr-only">{{ " " + model.name }}</span></BaseButton>
        <BaseButton v-else variant="secondary" size="sm" icon="play" :disabled="!canStart" @click="emit('start')">Start<span class="sr-only">{{ " " + model.name }}</span></BaseButton>
      </template>
      <BaseButton variant="ghost" size="sm" icon="edit" @click="emit('edit')">Edit<span class="sr-only">{{ " " + model.name }}</span></BaseButton>
    </div>
  </article>
</template>
