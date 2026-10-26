<script setup lang="ts">
import { computed } from "vue"
import { useRouter } from "vue-router"
import Callout from "../common/feedback/Callout.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ReadinessBanner from "./ReadinessBanner.vue"
import { AUDITOR_PROFILE } from "../../constants/netaudit"
import { toAutomationEdit } from "../../router/routes"

// Everything a network audit needs, shown where the audit is set up or read: what the computer is missing (the
// readiness banner) and whether this automation can reach the network at all. Nothing shows for other agents.
const props = defineProps<{ profile?: string; networkGrant?: string; automationId?: string; fixable?: boolean }>()
const emit = defineEmits<{ (e: "use-local-network"): void }>()

const router = useRouter()
const isAudit = computed(() => props.profile === AUDITOR_PROFILE)
// "No network" and "Internet only" both take the scan tool away; the other choices (and inheriting) leave it.
const cutOff = computed(() => props.networkGrant === "none" || props.networkGrant === "internet_only")
</script>

<template>
  <div v-if="isAudit" class="flex flex-col gap-2" data-test="audit-setup">
    <Callout v-if="cutOff" tone="warning" title="This automation can't reach your network">
      Its network access is set so the scan tool is switched off. Set it to “Local network only” and the audit can run.
      <template #actions>
        <BaseButton v-if="fixable" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('use-local-network')">Use “Local network only”</BaseButton>
        <BaseButton v-else-if="automationId" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="router.push(toAutomationEdit(automationId))">Edit automation</BaseButton>
      </template>
    </Callout>
    <ReadinessBanner />
  </div>
</template>
