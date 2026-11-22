<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import ChipInput from "../common/forms/ChipInput.vue"
import { topicEdit, topicFields } from "../../utils/newsroom/briefForm"
import {
  REGION_OPTIONS, TOPIC_FEEDS_HINT, TOPIC_FEEDS_LABEL, TOPIC_IGNORE_HINT, TOPIC_IGNORE_LABEL, TOPIC_NAME_HINT, TOPIC_NAME_LABEL, TOPIC_NEEDS_SOMETHING, TOPIC_REGION_LABEL,
  TOPIC_SEARCHES_HINT, TOPIC_SEARCHES_LABEL, TOPIC_SEARCHES_PLACEHOLDER, WATCH_NAME_MAX,
} from "../../constants/newsroom"
import type { BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// Create or edit one topic: a name, what it should find (chips), an optional website to follow, and the region and words to skip. There is nothing hidden
// behind "Advanced". The server checks everything again and says why it refuses (`error`).
const props = defineProps<{ topic: BriefTopic | null; saving: boolean; error: string }>()
const emit = defineEmits<{ (e: "save", edit: BriefTopicEdit): void; (e: "cancel"): void }>()

const f = ref(topicFields(props.topic))
const edit = computed(() => topicEdit(f.value, props.topic))
const hasSomething = computed(() => edit.value.queries.length + edit.value.sources.length > 0)
const canSave = computed(() => !props.saving && edit.value.name !== "" && hasSomething.value)
// A stored region the list does not offer (the assistant made it) stays selectable instead of being replaced.
const regions = computed(() => (REGION_OPTIONS.some((o) => o.value === f.value.region) ? REGION_OPTIONS : [...REGION_OPTIONS, { value: f.value.region, label: f.value.region }]))
function submit() {
  if (canSave.value) emit("save", edit.value)
}
</script>

<template>
  <form class="flex flex-col gap-4" @submit.prevent="submit">
    <FormField :label="TOPIC_NAME_LABEL" :hint="TOPIC_NAME_HINT">
      <template #default="{ id, describedBy }">
        <input :id="id" v-model="f.name" :aria-describedby="describedBy" type="text" :maxlength="WATCH_NAME_MAX" required class="form-control" />
      </template>
    </FormField>
    <FormField :label="TOPIC_SEARCHES_LABEL" :hint="TOPIC_SEARCHES_HINT">
      <template #default="{ id, describedBy }">
        <ChipInput :id="id" v-model="f.searches" :label="TOPIC_SEARCHES_LABEL" :described-by="describedBy" :placeholder="TOPIC_SEARCHES_PLACEHOLDER" />
      </template>
    </FormField>
    <FormField :label="TOPIC_FEEDS_LABEL" :hint="TOPIC_FEEDS_HINT">
      <template #default="{ id, describedBy }">
        <textarea :id="id" v-model="f.feeds" :aria-describedby="describedBy" rows="2" placeholder="https://example.com/feed.xml" class="form-control"></textarea>
      </template>
    </FormField>
    <p v-if="!hasSomething && f.name.trim()" role="status" class="m-0 text-[length:var(--text-small)] text-faint" data-test="topic-needs">{{ TOPIC_NEEDS_SOMETHING }}</p>
    <div class="grid gap-4 sm:grid-cols-2">
      <FormField :label="TOPIC_REGION_LABEL">
        <template #default="{ id }">
          <select :id="id" v-model="f.region" class="form-control"><option v-for="o in regions" :key="o.value" :value="o.value">{{ o.label }}</option></select>
        </template>
      </FormField>
      <FormField :label="TOPIC_IGNORE_LABEL" :hint="TOPIC_IGNORE_HINT">
        <template #default="{ id, describedBy }">
          <ChipInput :id="id" v-model="f.ignore" lower :label="TOPIC_IGNORE_LABEL" :described-by="describedBy" />
        </template>
      </FormField>
    </div>
    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
    <div class="flex gap-2">
      <BaseButton type="submit" size="sm" :disabled="!canSave">Save</BaseButton>
      <BaseButton variant="ghost" size="sm" @click="emit('cancel')">Cancel</BaseButton>
    </div>
  </form>
</template>
