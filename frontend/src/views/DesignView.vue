<script setup lang="ts">
import { computed } from 'vue'
import type { ResolvedTokens, TokenName } from '../types/theme'
import { useTheme } from '../composables/ui/useTheme'
import { evaluateContrast } from '../theme/contrastPairs'
import { PRESETS } from '../theme/presets'
import { TOKEN_REGISTRY } from '../theme/tokenRegistry'
import { PRODUCT_NAME } from '../config/brand'
import PrimitivesGallery from './design/PrimitivesGallery.vue'

// Dev-only living reference (plan D23): every token, its measured contrast in
// the active theme, the type scale and the motion tokens. Supersedes the
// Phase 0 static mockups; grows with each primitive in Phase 5.

const TYPE_SAMPLES: { token: TokenName; label: string; mono?: boolean }[] = [
  { token: 'text-display', label: 'Display — page titles' },
  { token: 'text-numeral', label: '48.6 tok/s', mono: true },
  { token: 'text-heading', label: 'Heading — panels and dialogs' },
  { token: 'text-body', label: 'Body — the default reading size' },
  { token: 'text-small', label: 'Small — metadata and table cells', mono: true },
  { token: 'text-micro', label: 'MICRO LABEL', mono: true },
]
const MOTION: TokenName[] = ['motion-fast', 'motion-base', 'motion-slow']

const theme = useTheme()
const colourTokens = (Object.keys(TOKEN_REGISTRY) as TokenName[]).filter((t) => TOKEN_REGISTRY[t].kind === 'colour')

const resolved = computed<ResolvedTokens>(() => ({
  ...PRESETS[theme.current.value.presetId].tokens,
  ...theme.current.value.overrides,
}))
const contrast = computed(() => evaluateContrast(resolved.value))
const failures = computed(() => contrast.value.filter((r) => !r.pass).length)
const following = computed(() => theme.selection.value === null)
</script>

<template>
  <main class="min-h-screen bg-canvas text-secondary">
    <div class="mx-auto flex max-w-6xl flex-col gap-10 px-4 py-8 md:px-10">
      <header class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p class="font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">
            {{ PRODUCT_NAME }} · design system · dev only
          </p>
          <h1 class="mt-1 text-[length:var(--text-display)] font-semibold text-primary">Tokens</h1>
        </div>
        <div class="flex flex-wrap gap-2" role="group" aria-label="Theme">
          <button
            v-for="preset in theme.presets"
            :key="preset.id"
            type="button"
            :data-test="`preset-${preset.id}`"
            :aria-pressed="theme.current.value.presetId === preset.id && !following"
            class="rounded-[var(--radius-sm)] border border-control px-3 py-1.5 font-mono text-[length:var(--text-small)] text-primary transition-colors duration-fast hover:bg-surface-hover aria-pressed:bg-surface-active"
            @click="theme.selectPreset(preset.id)"
          >
            {{ preset.label }}
          </button>
          <button
            type="button"
            :aria-pressed="following"
            class="rounded-[var(--radius-sm)] border border-control px-3 py-1.5 font-mono text-[length:var(--text-small)] text-muted transition-colors duration-fast hover:bg-surface-hover aria-pressed:bg-surface-active aria-pressed:text-primary"
            @click="theme.followSystem()"
          >
            Follow system
          </button>
        </div>
      </header>

      <section aria-labelledby="colour-heading">
        <h2 id="colour-heading" class="mb-3 font-mono text-[length:var(--text-small)] uppercase tracking-[var(--tracking-micro)] text-primary">
          01 ── Colour tokens
        </h2>
        <ul class="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-px border border-hairline bg-border-hairline">
          <li v-for="token in colourTokens" :key="token" data-test="swatch" class="flex items-center gap-3 bg-surface p-3">
            <span class="h-8 w-8 shrink-0 border border-hairline" :style="{ background: `rgb(var(--${token}))` }" aria-hidden="true" />
            <span class="min-w-0">
              <span class="block truncate font-mono text-[length:var(--text-small)] text-primary">--{{ token }}</span>
              <span class="block font-mono text-[length:var(--text-micro)] text-muted">{{ resolved[token] }}</span>
            </span>
          </li>
        </ul>
      </section>

      <section aria-labelledby="contrast-heading">
        <h2 id="contrast-heading" class="mb-3 font-mono text-[length:var(--text-small)] uppercase tracking-[var(--tracking-micro)] text-primary">
          02 ── Contrast · {{ theme.current.value.themeId }}
          <span :class="failures ? 'text-state-error' : 'text-state-success'">
            {{ failures ? `${failures} failing` : 'all pass' }}
          </span>
        </h2>
        <div class="overflow-x-auto border border-hairline">
          <table class="w-full font-mono text-[length:var(--text-small)]">
            <thead class="text-left text-muted">
              <tr class="border-b border-hairline">
                <th scope="col" class="px-3 py-2 font-normal">Sample</th>
                <th scope="col" class="px-3 py-2 font-normal">Foreground on background</th>
                <th scope="col" class="px-3 py-2 font-normal">Role</th>
                <th scope="col" class="px-3 py-2 text-right font-normal">Ratio</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in contrast"
                :key="`${row.fg}/${row.bg}`"
                data-test="contrast-row"
                :data-pass="row.pass"
                class="border-b border-hairline last:border-0"
              >
                <td class="px-3 py-1.5">
                  <span class="inline-block px-2" :style="{ color: `rgb(var(--${row.fg}))`, background: `rgb(var(--${row.bg}))` }">Aa</span>
                </td>
                <td class="px-3 py-1.5 text-primary">{{ row.fg }} <span class="text-muted">on</span> {{ row.bg }}</td>
                <td class="px-3 py-1.5 text-muted">{{ row.role }}</td>
                <td class="px-3 py-1.5 text-right tabular-nums" :class="row.pass ? 'text-primary' : 'text-state-error'">
                  {{ row.ratio.toFixed(2) }} <span class="text-muted">≥ {{ row.min }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section aria-labelledby="type-heading">
        <h2 id="type-heading" class="mb-3 font-mono text-[length:var(--text-small)] uppercase tracking-[var(--tracking-micro)] text-primary">
          03 ── Type scale
        </h2>
        <ul class="flex flex-col gap-4">
          <li v-for="sample in TYPE_SAMPLES" :key="sample.token" class="flex flex-wrap items-baseline gap-x-6 gap-y-1">
            <span class="w-32 shrink-0 font-mono text-[length:var(--text-micro)] text-muted">--{{ sample.token }}</span>
            <span
              :class="sample.mono ? 'font-mono' : 'font-sans'"
              class="text-primary"
              :style="{ fontSize: `var(--${sample.token})` }"
            >{{ sample.label }}</span>
          </li>
        </ul>
      </section>

      <section aria-labelledby="motion-heading">
        <h2 id="motion-heading" class="mb-3 font-mono text-[length:var(--text-small)] uppercase tracking-[var(--tracking-micro)] text-primary">
          04 ── Motion
        </h2>
        <p class="mb-3 text-[length:var(--text-small)] text-muted">Hover a bar to play its duration. Reduced motion zeroes all three.</p>
        <ul class="flex flex-col gap-2">
          <li v-for="token in MOTION" :key="token" class="group flex items-center gap-4">
            <span class="w-32 shrink-0 font-mono text-[length:var(--text-micro)] text-muted">--{{ token }}</span>
            <span class="relative h-2 flex-1 bg-bar-track">
              <span
                class="absolute inset-y-0 left-0 w-0 bg-accent-brand group-hover:w-full"
                :style="{ transition: `width var(--${token}) var(--ease-standard)` }"
              />
            </span>
          </li>
        </ul>
      </section>

      <PrimitivesGallery />
    </div>
  </main>
</template>
