# Plan: Consolidate banner logic into a single event-driven `AppBanner`

## Goal

Replace the three overlapping banner components (`BaseBanner`, `WarningBanner`,
`ErrorBanner`) and the inline `chat-error-banner` with one banner surface owned by
`App.vue`, fed by a typed event bus so any component can raise a banner with a
configured severity. Mirrors the existing `useToast` / `useConfirm` pattern.

## Design decisions (confirmed)

- **Single component:** `src/components/ui/AppBanner.vue` (matches `Toast.vue` /
  `ConfirmDialog.vue` location + naming). Renders one active banner with
  `severity`, `message`, and a `persistent` flag (no dismiss button when persistent).
- **Single event bus:** `src/composables/ui/useAppBanner.ts` — module-level
  `ref<AppBannerMessage | null>` + `show()` / `clear()`. Not a promise/callback
  like `useConfirm` because the banner is not awaitable.
- **Severity configured per message:** `AppBannerMessage = { severity: 'critical'
  | 'notice' | 'error'; message: string; persistent?: boolean }`. Host maps
  severity → color (amber / blue / red).
- **Dismissal:** per-message `persistent` flag. Model warning = persistent (sticky,
  re-evaluated on state refresh); transient errors = dismissable.
- **Model warning route:** `App.vue` keeps `computeModelBanner(state.value)` (in
  `composables/models/modelBanner.ts`). It is the *default* layer: written to the
  bus only when no transient event banner is active (events win), so a dismissable
  error is never clobbered by a state refresh. When the warning clears, the default
  layer is removed without touching an active event banner.
- **SVG convention:** inline SVGs replaced with `Icon` (loads from `assets/svg/`).
  Added `assets/svg/warning.svg` and `assets/svg/close.svg`.

## Status

### Completed
- [x] New `src/composables/ui/useAppBanner.ts` (event bus).
- [x] New `src/components/ui/AppBanner.vue` (single host; severity→color; reads bus).
- [x] New `src/assets/svg/warning.svg` and `src/assets/svg/close.svg` (Icon assets).
- [x] `src/App.vue`: remove `WarningBanner`; mount `<AppBanner />`; reconcile model
      warning into bus as default layer (`watch(state, ...)`, events win).
- [x] `src/composables/assistant/useAssistant.ts`: remove local `error` ref; route
      errors through `useAppBanner().show/clear`.
- [x] `src/composables/automation/useDispatcher.ts`: remove `error`/`clearError`;
      route dispatcher errors through `useAppBanner`.
- [x] `src/components/AgentIde/AgentIde.vue`: remove `ErrorBanner` import,
      `error`/`clearError` from `useDispatcher`, and `<ErrorBanner>` block.
- [x] `src/components/AgentIde/assistant/AssistantChat.vue`: remove `error` /
      `dismissError`; remove `:error` / `@dismiss-error` pass-through to ChatMessages.
- [x] `src/components/AgentIde/assistant/ChatMessages.vue`: remove `error` prop,
      `dismissError` emit, inline `chat-error-banner` block + styles.
- [x] Delete `src/components/common/BaseBanner.vue`,
      `src/components/common/WarningBanner.vue`,
      `src/components/AgentIde/common/ErrorBanner.vue`.
- [x] Unit test `src/__TESTS__/composables/ui/useAppBanner.test.ts` (show/clear,
      severity, persistent). `modelBanner.test.ts` unchanged.
- [x] Verification: `npm run build` (eslint + vue-tsc + vite) clean; `npm test`
      10 passed.

### Notes / open consideration
- Chat-level errors now surface as the global top banner (previously an inline
  banner inside the chat pane). Intentional per "single banner" consolidation.
  If inline-in-chat placement is desired, that is a deliberate exception to carve
  out later.
- `computeModelBanner` (model warning logic) remains in
  `composables/models/modelBanner.ts` — already abstracted, unchanged.

## Verification (run)
- `cd frontend && npm test` → 10 passed.
- `cd frontend && npm run build` → clean (eslint + vue-tsc + vite).
