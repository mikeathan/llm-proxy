<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import { topicEdit, topicFields } from "../../utils/newsroom/briefForm"
import {
  ADVANCED_LABEL, TOPIC_CATEGORY_LABEL, TOPIC_FEEDS_HINT, TOPIC_FEEDS_LABEL, TOPIC_IGNORE_HINT, TOPIC_IGNORE_LABEL, TOPIC_NAME_HINT, TOPIC_NAME_LABEL, TOPIC_NEEDS_SOMETHING,
  TOPIC_REGION_HINT, TOPIC_REGION_LABEL, TOPIC_SEARCHES_HINT, TOPIC_SEARCHES_LABEL, WATCH_NAME_MAX,
} from "../../constants/newsroom"
import type { BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// Create or edit one topic: a name, what it covers and the feeds to read. Region, category and the words to leave out are under Advanced. The server checks
// everything again and says why it refuses (`error`).
const props = defineProps<{ topic: BriefTopic | null; saving: boolean; error: string }>()
const emit = defineEmits<{ (e: "save", edit: BriefTopicEdit): void; (e: "cancel"): void }>()

const f = ref(topicFields(props.topic))
const edit = computed(() => topicEdit(f.value, props.topic))
const hasSomething = computed(() => edit.value.queries.length + edit.value.sources.length > 0)
const canSave = computed(() => !props.saving && edit.value.name !== "" && hasSomething.value)
function submit() {
  if (canSave.value) emit("save", edit.value)
}
</script>

<template>
  <form class="flex flex-col gap-3" @submit.prevent="submit">
    <FormField :label="TOPIC_NAME_LABEL" :hint="TOPIC_NAME_HINT">
      <template #default="{ id, describedBy }">
        <input :id="id" v-model="f.name" :aria-describedby="describedBy" type="text" :maxlength="WATCH_NAME_MAX" required class="form-control" />
      </template>
    </FormField>
    <FormField :label="TOPIC_SEARCHES_LABEL" :hint="TOPIC_SEARCHES_HINT">
      <template #default="{ id, describedBy }">
        <textarea :id="id" v-model="f.searches" :aria-describedby="describedBy" rows="3" class="form-control"></textarea>
      </template>
    </FormField>
    <FormField :label="TOPIC_FEEDS_LABEL" :hint="TOPIC_FEEDS_HINT">
      <template #default="{ id, describedBy }">
        <textarea :id="id" v-model="f.feeds" :aria-describedby="describedBy" rows="2" class="form-control"></textarea>
      </template>
    </FormField>
    <p v-if="!hasSomething && f.name.trim()" role="status" class="m-0 text-[length:var(--text-small)] text-faint" data-test="topic-needs">{{ TOPIC_NEEDS_SOMETHING }}</p>
    <details class="text-[length:var(--text-small)]">
      <summary class="cursor-pointer text-accent-brand">{{ ADVANCED_LABEL }}</summary>
      <div class="mt-2 flex flex-col gap-3">
        <FormField :label="TOPIC_CATEGORY_LABEL">
          <template #default="{ id }"><input :id="id" v-model="f.category" type="text" :maxlength="WATCH_NAME_MAX" class="form-control" /></template>
        </FormField>
        <FormField :label="TOPIC_REGION_LABEL" :hint="TOPIC_REGION_HINT">
          <template #default="{ id, describedBy }"><input :id="id" v-model="f.region" :aria-describedby="describedBy" type="text" maxlength="5" placeholder="en-GB" class="form-control" /></template>
        </FormField>
        <FormField :label="TOPIC_IGNORE_LABEL" :hint="TOPIC_IGNORE_HINT">
          <template #default="{ id, describedBy }"><input :id="id" v-model="f.ignore" :aria-describedby="describedBy" type="text" class="form-control" /></template>
        </FormField>
      </div>
    </details>
    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
    <div class="flex gap-2">
      <BaseButton type="submit" size="sm" :disabled="!canSave">Save</BaseButton>
      <BaseButton variant="ghost" size="sm" @click="emit('cancel')">Cancel</BaseButton>
    </div>
  </form>
</template>
