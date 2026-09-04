# systemd Service Setup

> **Automated:** `sudo ./setup.sh` opens a `dialog`/`whiptail` TUI (classic
> setup-screen panels; plain-ANSI fallback) with install / register-only /
> build-only / uninstall / **full purge** / preview options, and is idempotent —
> safe to re-run; it skips what's already done.

## Subcommands

| Command | Purpose |
|---|---|
| `install` | Build binary + register systemd service (full flow) |
| `register` | Register systemd only (binary already built) |
| `build` | Build the binary only (delegates to `scripts/build.sh`) |
| `service` | Service control: start / stop / restart / status / logs / follow |
| `access` | Grant the service user read access to external paths (ACLs; opt-in) |
| `uninstall` | Remove service + binary (asks about data; data kept by default) |
| `purge` | Full purge — typed `PURGE` confirmation behind a full-screen warning |
| `preview` | Dry-run preview of install |

Unattended flags (skip the TUI): `--install` `--uninstall` `--build` `--force`
`--dry-run` `--yes`.

## Service user profile (install)

The installer asks **who runs the service**, with an explanatory screen:

- **Dedicated account** (default, `llm-proxy`) — a locked system account that
  owns nothing but the data root. Strongest containment: a compromised proxy
  cannot read your home directory unless you grant specific paths. Because it
  cannot read outside, the installer then offers **External model & runtime
  paths** — model dir + llama-server binary — and applies read-only ACLs
  (`setfacl -R -m u:<user>:rX <path>`, traverse-only on parents up to `/`),
  so your home stays private (traverse, no listing). Answers are saved in
  `$SVC_ROOT/.setup-paths` (0600) to prefill future runs; also available
  standalone as `./setup.sh access`.
- **Your own account** (the invoking user) — zero grants ever needed; model
  dirs and binaries anywhere in your home just work. The trade-off, spelled
  out on the installer screen: the proxy and every agent session it spawns run
  with your full identity and can read everything you can (SSH keys,
  credentials, dev tree). Choose only on a personal, single-user machine.
- **Custom username** — another name; a locked system account is created
  (same flow as the dedicated profile, including the access-grant step).

Uninstall refuses to `userdel` the invoking user's account (self-profile
installs) and keeps the user whenever it still owns the data root.

## Running directly (dev, no systemd)

`./launch.sh` runs the binary in the foreground for development:

- Builds first if `backend/llm-proxy` is missing (via `scripts/build.sh`); warns if the embedded frontend assets (`backend/frontend_dist`) were never compiled.
- State root resolution, most specific wins:
  1. anything you export before calling — `LLM_PROXY_HOME=~/dev-state ./launch.sh`
  2. `.launch.env` in the repo root (generated/refreshed by `setup.sh` on every install; gitignored) — mirrors the installed service's data root, so dev and service agree on paths by default
  3. the app's implicit fallback, `~/.config/llm-proxy`
- Extra args pass straight through: `./launch.sh --data /tmp/other`.

Note: dev runs against the *default* home layout use a separate state world
from the service. `.launch.env` (pointing at the service's data root) is what
keeps them aligned; export your own `LLM_PROXY_HOME` to isolate dev runs.

## Manual reference

Subcommands and flags above are what the TUI automates; the manual steps
remain here as reference.

The unit lives at `docs/services/llm-proxy.service` and is hardened for a
dedicated unprivileged deployment: the service runs as `llm-proxy` with its
single root at `/var/lib/llm-proxy` (the app resolves one root via
`LLM_PROXY_HOME` — config, DB, logs, and workspaces all live under it).
`ProtectHome=read-only` + `ProtectSystem=strict` keep every other path
read-only; `ReadWritePaths` grants exactly one writable location.

## 1. Create the service account and root

```bash
sudo useradd --system --home-dir /var/lib/llm-proxy --shell /usr/sbin/nologin llm-proxy
sudo install -d -o llm-proxy -g llm-proxy -m 0700 /var/lib/llm-proxy
```

## 2. Install the binary and unit

```bash
sudo cp backend/llm-proxy /usr/local/bin/llm-proxy
sudo cp docs/services/llm-proxy.service /etc/systemd/system/llm-proxy.service
```

## 3. Point workspaces inside the writable root

In `/var/lib/llm-proxy/settings.yml` set:

```yaml
workspaces_dir: workspaces   # relative → resolves to /var/lib/llm-proxy/workspaces
```

Without this, the default workspace location falls back to `$HOME` (see
`backend/internal/platform/storage/manager.go`), which is read-only under
`ProtectHome` and will fail on first agent run.

## 4. Migrate existing state (single-root layout)

From a previous `~/.config/llm-proxy` deployment, as root:

```bash
cp -a /home/<olduser>/.config/llm-proxy/. /var/lib/llm-proxy/
chown -R llm-proxy:llm-proxy /var/lib/llm-proxy
```

## 5. Enable and start

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now llm-proxy.service
journalctl -u llm-proxy.service -f
```

## Notes

- `Restart=on-failure` + `StartLimitBurst=5` prevent infinite crash loops; a
  persistent failure stops the unit after 5 attempts within 5 minutes.
- `MemoryDenyWriteExecute=true` is safe for the Go binary; remove it only if a
  future embedded runtime needs executable-mapping memory.
- The service listens on an unprivileged port (:4001 default), so it needs no
  capabilities (`CapabilityBoundingSet=` is empty by design).
- Check effective hardening any time with:
  `systemd-analyze security llm-proxy.service`
