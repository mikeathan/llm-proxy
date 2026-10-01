<script setup lang="ts">
import type { ButtonSize, ButtonVariant } from '../../../types/ui';
import Icon from '../../icons/Icon.vue';

// The one button (Phase 5 primitive): mono label, hairline control border;
// primary is ink with the brand offset. Icon-only buttons take `label` as
// their accessible name (a source-scanning test enforces it).
interface Props {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  disabled?: boolean;
  icon?: string;
  iconOnly?: boolean;
  /** Accessible name and tooltip; required when iconOnly. */
  label?: string;
  type?: 'button' | 'submit' | 'reset';
  className?: string;
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'primary',
  size: 'md',
  type: 'button',
  className: ''
});

const VARIANT_CLASS: Record<ButtonVariant, string> = {
  primary: 'border-text-primary bg-text-primary text-inverse offset-brand hover:enabled:bg-text-secondary active:enabled:translate-x-[var(--offset-control)] active:enabled:translate-y-[var(--offset-control)] active:enabled:shadow-none',
  secondary: 'border-control bg-transparent text-primary hover:enabled:bg-surface-hover',
  ghost: 'border-transparent bg-transparent text-muted hover:enabled:bg-surface-hover hover:enabled:text-primary',
  danger: 'border-state-error/70 bg-transparent text-state-error hover:enabled:bg-state-error/10',
};

const SIZE_CLASS: Record<ButtonSize, string> = {
  sm: 'h-[26px] px-2.5',
  md: 'h-[30px] px-3',
  lg: 'h-9 px-4',
};

const ICON_ONLY_SIZE_CLASS: Record<ButtonSize, string> = {
  sm: 'h-[26px] w-[26px]',
  md: 'h-[30px] w-[30px]',
  lg: 'h-9 w-9',
};

const BASE_CLASS = 'inline-flex flex-none items-center justify-center gap-1.5 whitespace-nowrap rounded-[var(--radius-sm)] border font-mono text-[length:var(--text-small)] font-medium transition-[background-color,border-color,color,transform] duration-fast ease-standard focus-visible:outline-none focus-visible:ring-2 disabled:cursor-not-allowed disabled:opacity-45';
</script>

<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :aria-busy="loading ? 'true' : undefined"
    :aria-label="iconOnly ? label : undefined"
    :title="iconOnly ? label : undefined"
    :data-variant="variant"
    :class="[BASE_CLASS, VARIANT_CLASS[variant], iconOnly ? ICON_ONLY_SIZE_CLASS[size] : SIZE_CLASS[size], className]"
  >
    <Icon v-if="loading" name="spinner" size="sm" />
    <Icon v-else-if="icon" :name="icon" size="sm" />
    <slot v-if="!iconOnly"></slot>
  </button>
</template>
