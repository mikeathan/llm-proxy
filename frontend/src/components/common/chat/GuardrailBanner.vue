<script setup lang="ts">
import { ref } from "vue";
import type { GuardrailBlockedPayload } from "../../../types";
import BaseButton from "../buttons/BaseButton.vue";

// A guardrail blocked a tool call and the run waits for the operator. The
// banner only asks; `submit` (the owner's submitDecision) sends the decision —
// the one place it is sent. While it is in flight every choice is locked, and a
// failure leaves the banner ready to try again.
const props = defineProps<{
  decision: GuardrailBlockedPayload;
  submit: (allow: boolean, persist: boolean) => Promise<void>;
}>();

const SEND_FAILED = "Could not send the decision — try again.";

const submitting = ref(false);
const failed = ref(false);

async function decide(allow: boolean, persist: boolean) {
  if (submitting.value) return;
  submitting.value = true;
  failed.value = false;
  try {
    await props.submit(allow, persist);
  } catch {
    failed.value = true;
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div role="alert" class="flex flex-col gap-3 border-l-2 border-state-running bg-state-running/[0.08] px-4 py-3">
    <p class="m-0 font-mono text-[length:var(--text-small)] font-semibold uppercase tracking-[var(--tracking-micro)] text-state-running">
      Approval needed — a guardrail blocked this step
    </p>
    <dl class="m-0 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-1 text-[length:var(--text-small)]">
      <dt class="text-muted">Tool</dt>
      <dd class="m-0 font-mono text-primary">{{ decision.tool }}</dd>
      <dt class="text-muted">Reason</dt>
      <dd class="m-0 text-secondary">{{ decision.reason }}</dd>
      <dt class="text-muted">Category</dt>
      <dd class="m-0 font-mono text-secondary">{{ decision.category }}</dd>
    </dl>
    <div class="flex flex-wrap gap-2">
      <BaseButton variant="primary" size="sm" :disabled="submitting" @click="decide(true, true)">Allow and remember</BaseButton>
      <BaseButton variant="secondary" size="sm" :disabled="submitting" @click="decide(true, false)">Allow once</BaseButton>
      <BaseButton variant="danger" size="sm" :disabled="submitting" @click="decide(false, false)">Deny</BaseButton>
    </div>
    <p v-if="failed" class="m-0 text-[length:var(--text-small)] text-state-error">{{ SEND_FAILED }}</p>
  </div>
</template>
