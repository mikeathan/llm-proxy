<script setup lang="ts">
import { useToast } from "../../composables/useToast";
import { TOAST_SUCCESS, TOAST_ERROR, TOAST_WARNING, TOAST_INFO } from "../../constants/icons";
import type { ToastType } from "../../types/ui";
import BaseButton from "../common/buttons/BaseButton.vue";

// Transient notices, announced through one polite live region; an error is
// also role="alert" so it is announced at once. Each can be dismissed by its
// named button or by clicking it.
const { toasts, remove } = useToast();

const MARK: Record<ToastType, string> = { success: TOAST_SUCCESS, error: TOAST_ERROR, warning: TOAST_WARNING, info: TOAST_INFO };
const TONE: Record<ToastType, string> = {
  success: "border-l-state-success text-state-success",
  error: "border-l-state-error text-state-error",
  warning: "border-l-state-running text-state-running",
  info: "border-l-accent-info text-accent-info-text",
};
</script>

<template>
  <div aria-live="polite" class="pointer-events-none fixed bottom-6 right-6 z-[9999] flex max-w-[calc(100vw-3rem)] flex-col gap-3">
    <TransitionGroup name="toast">
      <div
        v-for="toast in toasts"
        :key="toast.id"
        :role="toast.type === 'error' ? 'alert' : undefined"
        :class="[
          'pointer-events-auto flex w-[360px] max-w-full cursor-pointer items-center gap-3 rounded-[var(--radius-sm)] border border-l-2 border-strong bg-surface px-3 py-2.5',
          TONE[toast.type],
        ]"
        @click="remove(toast.id)"
      >
        <span aria-hidden="true" class="flex-none font-mono text-[length:var(--text-small)] font-semibold">{{ MARK[toast.type] }}</span>
        <span class="min-w-0 flex-1 break-words text-[length:var(--text-small)] text-primary">{{ toast.message }}</span>
        <BaseButton variant="ghost" size="sm" icon="close" icon-only label="Dismiss notification" @click.stop="remove(toast.id)" />
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: opacity var(--motion-base) var(--ease-standard), transform var(--motion-base) var(--ease-standard);
}
.toast-enter-from {
  opacity: 0;
  transform: translateY(8px);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(16px);
}
.toast-move {
  transition: transform var(--motion-base) var(--ease-standard);
}
</style>
