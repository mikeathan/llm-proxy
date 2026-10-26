<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import Callout from "../common/feedback/Callout.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useScanScope } from "../../composables/netaudit/useScanScope"
import { explainFailure } from "../../utils/netaudit/explainFailure"
import { toAutomationEdit, toSettings } from "../../router/routes"

// A failed run, in plain words with the one thing to do. The original message stays one tap away, and a failure this
// does not know is shown exactly as it came.
const props = defineProps<{ error: string; automationId?: string }>()

const router = useRouter()
const scope = useScanScope()
const explanation = computed(() => explainFailure(props.error))
const showRaw = ref(false)
const allowed = computed(() => scope.ranges.value.map((r) => r.cidr).join(", "))

onMounted(() => {
  if (explanation.value?.showsAllowedNetworks) void scope.load()
})

function fix() {
  if (explanation.value?.fix === "network-settings") void router.push(toSettings("network-scan"))
  else if (explanation.value?.fix === "edit-automation" && props.automationId) void router.push(toAutomationEdit(props.automationId))
}
const fixLabel = computed(() => (explanation.value?.fix === "network-settings" ? "Open network settings" : "Edit automation"))
const hasFix = computed(() => explanation.value?.fix === "network-settings" || (explanation.value?.fix === "edit-automation" && !!props.automationId))
</script>

<template>
  <div v-if="explanation" class="flex flex-col gap-2" data-test="failure">
    <Callout tone="error" :title="explanation.title">
      <p class="m-0">{{ explanation.body }}</p>
      <p v-if="explanation.showsAllowedNetworks && allowed" class="m-0 mt-1">You allowed: <span class="break-all font-mono text-primary">{{ allowed }}</span></p>
      <template #actions>
        <BaseButton v-if="hasFix" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="fix">{{ fixLabel }}</BaseButton>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :aria-expanded="showRaw" @click="showRaw = !showRaw">{{ showRaw ? "Hide details" : "Show details" }}</BaseButton>
      </template>
    </Callout>
    <pre v-if="showRaw" class="m-0 whitespace-pre-wrap break-words font-mono text-[length:var(--text-small)] text-muted">{{ error }}</pre>
  </div>
  <pre v-else class="m-0 whitespace-pre-wrap break-words font-mono text-[length:var(--text-small)] text-state-error">{{ error }}</pre>
</template>
