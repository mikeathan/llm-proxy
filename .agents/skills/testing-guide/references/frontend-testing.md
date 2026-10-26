# Testing — Patterns, Tools & Strategies — Frontend Testing (Vitest + Playwright)

Reference for the [`testing-guide`](../SKILL.md) skill, loaded on demand. You are writing or running frontend unit or visual tests.

## Frontend testing (Vitest + Playwright)

Tests mirror the source tree in `frontend/src/__TESTS__/`. `frontend/vitest.config.ts` defines two projects:
**`unit`** (node, `*.test.ts`) and **`component`** (happy-dom + Vue plugin, `*.component.test.ts`) — a test that
mounts or imports a `.vue` file, or navigates the router (lazy route chunks), must be a `.component.test.ts`.
`npm run build` runs `npm run lint` (ESLint + palette ratchet + contrast gate) → `vue-tsc -b` → `vite build`.
`npm run test:visual` drives installed Chrome against the Vite dev server (`frontend/playwright.config.ts`);
add `-- --update-snapshots` only to accept an intended change.

Gotchas (each cost a debugging round):
- **Vitest stubs CSS imports, even `?raw`** — `tokens.css` is let through per project via `css.include`.
- The node project has no `localStorage` — use `__TESTS__/helpers/memoryStorage.ts` + `vi.stubGlobal`. Other
  helpers: `fieldByLabel.ts` (find a control by label; survives markup changes), `fakeMatchMedia.ts`.
- ES2020 target (`tsconfig.app.json`) + `noUncheckedIndexedAccess` (inherited from `@vue/tsconfig`): no `.at()` / `Object.hasOwn`; indexed access is `T | undefined`.
- happy-dom does not submit a form when its submit button is clicked — `form.trigger('submit')`.
- `onBeforeRouteLeave` registers only inside a `RouterView` — mount a wrapper rendering one, then `vi.waitFor`
  the lazy page. A route navigation settles after its chunk loads (`vi.waitFor`, not `flushPromises`);
  `router.resolve()` does not follow redirects (`router.push` + `currentRoute`).
- Route views read `composables/ui/useDestinationRoute.ts`, not `useRoute()` — with `out-in` transitions and
  keep-alive, a leaving view otherwise sees the *next* route.
- `structuredClone` throws on reactive props — copy plain prop data with JSON.

**Checking the real app (isolated backend):** the binary embeds `frontend_dist` at compile time, so run
`npm run build` and rebuild the binary before looking at UI changes. Build into a scratch dir and start it
**with cwd = that dir**: `cd <scratch> && ./lp-bin --data <scratch>/home`. Never start it with cwd `backend/` —
it then loads `backend/.env.development` (`internal/platform/env/env.go`) and points at the user's real
data. Confirm `GET /admin/api/dispatcher/workspaces` returns your fixtures before any write. To stop it, read
the PID's command line from `lsof -tiTCP:4001 -sTCP:LISTEN` first and kill only what you started. Ad-hoc
Playwright scripts use `channel: 'chrome'`; inject a theme with localStorage `admin-ui:theme-selection` =
`{"version":1,"data":{"kind":"preset","id":"retro-dark"}}` (plus `admin-ui:theme-applied`, see `e2e/design.visual.spec.ts`).

---
