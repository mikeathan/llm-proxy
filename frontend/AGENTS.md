# frontend/AGENTS.md

This folder is part of the repository whose contract is [`../AGENTS.md`](../AGENTS.md) — read that first; it
outranks this file. This file only adds what applies to the Vue 3 app under `frontend/`:

- Load the mandatory Vue rules: [`../.agents/rules/frontend-vue-engineer.md`](../.agents/rules/frontend-vue-engineer.md).
- Find your subsystem's hub in [`../docs/subsystems/`](../docs/subsystems/README.md); the UI hub is
  [`admin-ui.md`](../docs/subsystems/admin-ui.md).
- After each meaningful edit: `npm run build` (lint runs inside it). Before finishing: `npm test` and
  `npm run build`, plus `npm run test:visual` when UI, theme or tokens change.
- A new settings tab follows [`../docs/architecture/checklists.md`](../docs/architecture/checklists.md#adding-a-frontend-settings-tab-checklist).
