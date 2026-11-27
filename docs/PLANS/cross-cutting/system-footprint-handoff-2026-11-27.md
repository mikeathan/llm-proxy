---
status: partial
last_updated: 2026-11-27
specs: [SPEC-003]
---

# Handoff: System stats "This service" (footprint) and the models visual baselines

**Branch:** `system-footprint-and-visual-fix`, from `origin/main` (309f0f75). **Nothing is committed or pushed.** Push with
`git push -u origin system-footprint-and-visual-fix` (a bare push would target main, the upstream it was created from).
`backend/internal/transport/http/frontend_dist/index.html` is rebuilt and belongs in the commit.

## Built
- **Four `models.visual` baselines regenerated** (desktop `models-list-editor`; mobile `models-cards`, `models-provider`, `models-this-computer`). Cause measured: `html { scrollbar-gutter: stable }` reserves 11 px in this Chrome. Why those four were taken without it is unknown.
- **`GET /admin/api/metrics/footprint`** (`backend/internal/platform/metrics/footprint.go`, handler `AdminFootprintHandler`): the service and the active model server, CPU / RSS / threads / open files / uptime / GPU memory, plus Go runtime figures. Read only while the panel is open; cached 2 s; two pids, no process-table walk.
- **Panel section "This service"** (`HostStats.vue`, `useFootprint.ts`, `summariseFootprint`); `PopoverPanel` now scrolls inside itself when taller than the screen.
- SPEC-003 2.80, learnings entry in `docs/learnings/admin-ui.md`.

## Checks run (macOS arm64)
`go build ./...`, `go test ./...` (all ok), `go run ./tools/check-complexity/` (no new violations), `GOOS=linux go vet` on the metrics package, `npm test` (3734), `npm run build`, `npm run test:visual` (470 passed, before the PopoverPanel scroll change), `./scripts/check-agent-harness.sh` (pass). Live: isolated binary, first read `cpu_percent: null`, second 0.19 %; a busy `yes` process read 99.6 %; panel looked at 1440 px and 800 px.
**Not run:** anything on Linux; the panel on a phone (below 640 px the strip is hidden unless a model is generating, so the sheet opens only then; the sheet already scrolls to 80 vh); the model row with a real running model.

## Left (owner, on vertex)
1. Deploy, open System stats with a model running. Check the model row: RSS about the llama-server size, CPU against `top`, **GPU memory** from DRM fdinfo (`drm-memory-vram` / `drm-memory-gtt`; `cat /proc/<pid>/fdinfo/* | grep drm-`). If the GPU is an APU most of it shows as "shared" (GTT).
2. If the GPU is NVIDIA: the row falls back to `nvidia-smi --query-compute-apps`.
3. Open files should show on Linux (it is omitted on macOS).
4. Decide whether a build without cgo on macOS matters (gopsutil then runs `ps` per figure; only while the panel is open).

## Traps
- A DRM client with no memory line is not a reading (Intel names its lines `drm-total-*`; not read).
- CPU % uses cumulative CPU time, never `Process.Percent()`.

## First file to open
`backend/internal/platform/metrics/footprint.go`
