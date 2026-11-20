<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import FormField from "../common/forms/FormField.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import { compactInterval } from "../../utils/newsroom/interval"
import {
  ADD_SOURCE_LABEL, DEFAULT_INTERVAL, INTERVAL_OPTIONS, MAX_SOURCES, REMOVE_SOURCE_LABEL, SOURCE_INSTANCE_HINT, SOURCE_INSTANCE_LABEL, SOURCE_KIND_INFO,
  SOURCE_KIND_LABEL, SOURCE_KIND_OPTIONS, SOURCE_TARGET_MAX, TOPIC_CHANNEL_HINT, TOPIC_CHANNEL_LABEL, TOPIC_INTERVAL_LABEL, TOPIC_NAME_LABEL, TOPIC_NAME_HINT, TOPIC_FORM_INTRO, TOPIC_NAME_MAX,
  TOPIC_ON_LABEL, TOPIC_SOURCES_LABEL,
} from "../../constants/newsroom"
import type { ChoiceOption } from "../../types/ui"
import type { NewsroomSource, NewsroomTopic, NewsroomTopicEdit, SourceKind } from "../../types/newsroom"

// Create or edit one topic. The server checks everything again and says why it refuses (`error`); this form only keeps an obviously empty one from
// being sent.
const props = defineProps<{ topic: NewsroomTopic | null; channels: ChoiceOption[]; saving: boolean; error: string }>()
const emit = defineEmits<{ (e: "save", edit: NewsroomTopicEdit): void; (e: "cancel"): void }>()

const blankSource = (): NewsroomSource => ({ kind: "feed", target: "" })
const name = ref(props.topic?.name ?? "")
const interval = ref(props.topic ? compactInterval(props.topic.interval) : DEFAULT_INTERVAL)
const channel = ref(props.topic?.channel ?? "")
const enabled = ref(props.topic?.enabled ?? true)
const sources = ref<NewsroomSource[]>(props.topic?.sources.map((s) => ({ ...s })) ?? [blankSource()])

// A stored interval the picker does not list (the assistant or an older version made it) stays selectable instead of being replaced.
const intervalOptions = computed<ChoiceOption[]>(() =>
  INTERVAL_OPTIONS.some((o) => o.value === interval.value) ? INTERVAL_OPTIONS : [...INTERVAL_OPTIONS, { value: interval.value, label: `Every ${interval.value}` }],
)
// A channel that is no longer switched on is kept visible so saving does not silently drop it.
const channelOptions = computed<ChoiceOption[]>(() =>
  props.channels.some((o) => o.value === channel.value) ? props.channels : [...props.channels, { value: channel.value, label: `${channel.value} (not switched on)` }],
)

const filled = computed(() => sources.value.filter((s) => s.target.trim() !== ""))
const canSave = computed(() => !props.saving && name.value.trim() !== "" && filled.value.length > 0)

function setKind(source: NewsroomSource, kind: string) {
  source.kind = kind as SourceKind
  if (source.kind !== "mastodon") delete source.instance
}

function submit() {
  if (!canSave.value) return
  emit("save", {
    name: name.value.trim(),
    interval: interval.value,
    channel: channel.value,
    enabled: enabled.value,
    sources: filled.value.map((s) => ({ kind: s.kind, target: s.target.trim(), ...(s.instance?.trim() ? { instance: s.instance.trim() } : {}), ...(s.label ? { label: s.label } : {}) })),
  })
}
</script>

<template>
  <form class="flex flex-col gap-3" @submit.prevent="submit">
    <p class="m-0 text-[length:var(--text-small)] text-secondary">{{ TOPIC_FORM_INTRO }}</p>
    <FormField :label="TOPIC_NAME_LABEL" :hint="TOPIC_NAME_HINT">
      <template #default="{ id, describedBy }">
        <input :id="id" v-model="name" :aria-describedby="describedBy" type="text" :maxlength="TOPIC_NAME_MAX" required class="form-control" />
      </template>
    </FormField>

    <fieldset class="m-0 flex min-w-0 flex-col gap-3 border-0 p-0">
      <legend class="mb-1 p-0 text-[length:var(--text-small)] font-medium text-secondary">{{ TOPIC_SOURCES_LABEL }}</legend>
      <div v-for="(source, i) in sources" :key="i" class="flex flex-col gap-2 border border-hairline p-3">
        <div class="flex flex-wrap items-end gap-2">
          <SelectInput :model-value="source.kind" :options="SOURCE_KIND_OPTIONS" :label="SOURCE_KIND_LABEL" @update:model-value="setKind(source, $event)" />
          <BaseButton v-if="sources.length > 1" variant="ghost" size="sm" :label="REMOVE_SOURCE_LABEL" icon="trash" icon-only @click="sources.splice(i, 1)" />
        </div>
        <FormField :label="SOURCE_KIND_INFO[source.kind].targetLabel" :hint="SOURCE_KIND_INFO[source.kind].hint">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="source.target" :aria-describedby="describedBy" type="text" :maxlength="SOURCE_TARGET_MAX" class="form-control" />
          </template>
        </FormField>
        <FormField v-if="source.kind === 'mastodon'" :label="SOURCE_INSTANCE_LABEL" :hint="SOURCE_INSTANCE_HINT">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="source.instance" :aria-describedby="describedBy" type="text" class="form-control" />
          </template>
        </FormField>
      </div>
      <BaseButton v-if="sources.length < MAX_SOURCES" variant="secondary" size="sm" icon="plus" class-name="self-start" @click="sources.push(blankSource())">{{ ADD_SOURCE_LABEL }}</BaseButton>
    </fieldset>

    <div class="flex flex-wrap gap-3">
      <FormField :label="TOPIC_INTERVAL_LABEL">
        <template #default="{ id }">
          <select :id="id" v-model="interval" class="form-control w-fit">
            <option v-for="o in intervalOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </template>
      </FormField>
      <FormField :label="TOPIC_CHANNEL_LABEL" :hint="TOPIC_CHANNEL_HINT">
        <template #default="{ id, describedBy }">
          <select :id="id" v-model="channel" :aria-describedby="describedBy" class="form-control w-fit">
            <option v-for="o in channelOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </template>
      </FormField>
    </div>

    <BaseToggle v-model="enabled" :label="TOPIC_ON_LABEL" />

    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
    <div class="flex gap-2">
      <BaseButton type="submit" :disabled="!canSave" :loading="saving">Save</BaseButton>
      <BaseButton variant="ghost" @click="emit('cancel')">Cancel</BaseButton>
    </div>
  </form>
</template>
