<script setup lang="ts">
import type { GuardrailBlockedPayload } from "../../types";

const props = defineProps<{
  decision: GuardrailBlockedPayload;
}>();

const emit = defineEmits<{
  allow: [persist: boolean];
  deny: [];
}>();

import { ApiService } from "../../services/api";

const submitting = ref(false);

const handleAllow = async (persist: boolean) => {
  submitting.value = true;
  try {
    await ApiService.submitGuardrailDecision(props.decision.decision_id, true, persist);
    emit("allow", persist);
  } catch (err) {
    console.error("Failed to submit guardrail decision", err);
  } finally {
    submitting.value = false;
  }
};

const handleDeny = async () => {
  submitting.value = true;
  try {
    await ApiService.submitGuardrailDecision(props.decision.decision_id, false, false);
    emit("deny");
  } catch (err) {
    console.error("Failed to submit guardrail decision", err);
  } finally {
    submitting.value = false;
  }
};

import { ref } from "vue";
</script>

<template>
  <div class="guardrail-banner">
    <div class="approval-header">
      <span class="approval-icon">🛑</span>
      <span class="approval-title">Guardrail Blocked — Action Required</span>
    </div>
    <div class="approval-details">
      <div><strong>Tool:</strong> {{ decision.tool }}</div>
      <div><strong>Reason:</strong> {{ decision.reason }}</div>
      <div><strong>Category:</strong> {{ decision.category }}</div>
    </div>
    <div class="approval-actions">
      <button class="btn-approve" :disabled="submitting" @click="handleAllow(true)">
        Allow &amp; Remember
      </button>
      <button class="btn-approve-once" :disabled="submitting" @click="handleAllow(false)">
        Allow Once
      </button>
      <button class="btn-deny" :disabled="submitting" @click="handleDeny">
        Deny
      </button>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.guardrail-banner {
  @apply p-4 bg-amber-900/30 border border-amber-500/50 rounded-lg animate-in fade-in slide-in-from-top-2;
}

.approval-header {
  @apply flex items-center gap-2 mb-3;
}

.approval-icon {
  @apply text-lg;
}

.approval-title {
  @apply text-amber-300 font-bold text-sm uppercase tracking-wide;
}

.approval-details {
  @apply text-[11px] text-amber-100/80 space-y-1 mb-3 pl-6 border-l border-amber-500/30 py-1;
}

.approval-actions {
  @apply flex gap-2 flex-wrap;
}

.btn-approve {
  @apply px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-[11px] font-bold rounded transition-colors uppercase tracking-wide;
}

.btn-approve-once {
  @apply px-3 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-[11px] font-bold rounded transition-colors uppercase tracking-wide;
}

.btn-deny {
  @apply px-3 py-1.5 bg-red-600 hover:bg-red-500 disabled:opacity-50 text-white text-[11px] font-bold rounded transition-colors uppercase tracking-wide;
}
</style>
