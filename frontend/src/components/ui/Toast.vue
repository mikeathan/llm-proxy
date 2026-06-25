<script setup lang="ts">
import { useToast } from "../../composables/useToast";
import { TOAST_SUCCESS, TOAST_ERROR, TOAST_WARNING, TOAST_INFO, TOAST_CLOSE } from "../../constants/icons";

const { toasts, remove } = useToast();
</script>

<template>
  <div class="toast-container">
    <TransitionGroup name="toast">
      <div
        v-for="toast in toasts"
        :key="toast.id"
        class="toast-item"
        :class="`toast-item--${toast.type}`"
        @click="remove(toast.id)"
      >
        <div class="toast-icon">
          <span v-if="toast.type === 'success'">{{ TOAST_SUCCESS }}</span>
          <span v-else-if="toast.type === 'error'">{{ TOAST_ERROR }}</span>
          <span v-else-if="toast.type === 'warning'">{{ TOAST_WARNING }}</span>
          <span v-else>{{ TOAST_INFO }}</span>
        </div>
        <div class="toast-content">
          {{ toast.message }}
        </div>
        <button class="toast-close" @click.stop="remove(toast.id)">{{ TOAST_CLOSE }}</button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped lang="postcss">
.toast-container {
  @apply fixed bottom-6 right-6 flex flex-col gap-3 pointer-events-none;
  z-index: 9999;
}

.toast-item {
  @apply pointer-events-auto flex items-center gap-3 px-4 py-3 rounded-xl shadow-2xl border
         min-w-[300px] max-w-md cursor-pointer transition-all duration-300;
}

.toast-item--success {
  @apply bg-emerald-950/90 border-emerald-500/30 text-emerald-200;
}

.toast-item--error {
  @apply bg-rose-950/90 border-rose-500/30 text-rose-200;
}

.toast-item--warning {
  @apply bg-amber-950/90 border-amber-500/30 text-amber-200;
}

.toast-item--info {
  @apply bg-blue-950/90 border-blue-500/30 text-blue-200;
}

.toast-icon {
  @apply flex-shrink-0 w-6 h-6 flex items-center justify-center rounded-full bg-white/10 font-bold text-sm;
}

.toast-content {
  @apply flex-1 text-sm font-medium leading-relaxed;
}

.toast-close {
  @apply opacity-50 hover:opacity-100 text-lg leading-none transition-opacity;
}

/* Animations */
.toast-enter-active,
.toast-leave-active {
  @apply transition-all duration-500;
  transition-timing-function: cubic-bezier(0.23, 1, 0.32, 1);
}

.toast-enter-from {
  @apply opacity-0 translate-y-4 scale-90 blur-sm;
}

.toast-leave-to {
  @apply opacity-0 translate-x-8 scale-95;
}

.toast-move {
  @apply transition-transform duration-500;
  transition-timing-function: cubic-bezier(0.23, 1, 0.32, 1);
}
</style>
