# Plan: Reasoning inset auto-expand while running, collapse on done

## Goal

Make the reasoning/tool inset default to **expanded while a turn is running** and
**collapsed once it finishes**, consistently across the Assistant chat and
Automation run views — sharing one implementation instead of the duplicated,
partially-broken logic.

## Root cause

- `AssistantChat.vue` drove auto expand/collapse off the `loading` ref, but
  `loading` flickers: `sendMessage()` sets it `true`, then the detached run's
  POST resolves (202) and `finally` sets it `false` *before* any reasoning
  streams. The `watch(loading)` collapsed-all on that `false` edge, so streaming
  reasoning stayed hidden behind the chevron for the whole run.
- `AutomationDetails.vue` had inset state + manual toggle but no phase watcher,
  so runs stayed expanded forever (never auto-collapsed on completion).
- Both files duplicated the same `insetCollapsed` / `isInsetCollapsed` /
  `toggleInset` state with different behavior.

## Design decisions

- **Shared composable** `src/composables/ui/useTurnInset.ts`, consumed by both
  `AssistantChat.vue` and `AutomationDetails.vue`. Both views drive the same
  `useMessageBuilder` phase signal (`idle → thinking/working/generating → done`),
  so one watcher covers both.
- **Phase-driven (not loading-driven):** expand the active (last) turn's inset
  on the run-start transition (`idle → running`), collapse it on `done`.
- **Manual toggle respected mid-run:** only the `idle → running` hop auto-expands;
  a user-collapsed inset stays collapsed through `thinking → working → generating`.

## Status

### Completed
- [x] New `src/composables/ui/useTurnInset.ts` (inset state + phase watcher).
- [x] `src/composables/ui/index.ts`: export `useTurnInset`.
- [x] `AssistantChat.vue`: use composable; remove local inset state,
      `watch(loading)`, and `watch(phase)` (superseded by composable).
- [x] `AutomationDetails.vue`: use composable; remove local inset state + manual
      toggle (now auto-collapses on done like chat).
- [x] Unit test `src/__TESTS__/composables/ui/useTurnInset.test.ts` (expand on
      run start, collapse on done, manual-toggle respected mid-run, collapse-all,
      empty-turns no-op).
- [x] Verification: `npm test` 25 passed; `npm run build` clean.

## Verification (run)
- `cd frontend && npm test` → 25 passed.
- `cd frontend && npm run build` → clean (vue-tsc + vite).
