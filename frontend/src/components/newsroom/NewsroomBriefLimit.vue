<script setup lang="ts">
import { computed, ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import { MODEL_CALLS_LABEL, MODEL_LIMIT_HINT, modelCallsProblem } from "../../constants/newsroom"
import type { BriefSettings } from "../../types/newsroom"

// The daily limit of model calls: the Newsroom's watches and its briefs share it. An empty box means the default. The server checks the number again.
const props = defineProps<{ settings: BriefSettings; saving: boolean; save: (calls: number) => Promise<boolean> }>()

// The box shows the owner's setting, and nothing when they have not set one.
const shown = (s: BriefSettings): string => (s.model_calls_per_day > 0 ? String(s.model_calls_per_day) : "")
const text = ref(shown(props.settings))
watch(() => props.settings, (s) => (text.value = shown(s)))

const calls = computed(() => (text.value.trim() === "" ? 0 : Number(text.value)))
const problem = computed(() => modelCallsProblem(text.value, props.settings.max))
const changed = computed(() => calls.value !== props.settings.model_calls_per_day)
const canSave = computed(() => !props.saving && changed.value && problem.value === "")
</script>

<template>
  <form class="flex flex-wrap items-end gap-3" @submit.prevent="canSave && save(calls)">
    <FormField :label="MODEL_CALLS_LABEL" :hint="MODEL_LIMIT_HINT(settings.default, settings.max)" :error="problem">
      <template #default="{ id, describedBy, invalid }">
        <input :id="id" v-model="text" :aria-describedby="describedBy" :aria-invalid="invalid" type="text" inputmode="numeric" :placeholder="String(settings.default)" class="form-control w-32" />
      </template>
    </FormField>
    <BaseButton type="submit" size="sm" :disabled="!canSave">Save</BaseButton>
  </form>
</template>
