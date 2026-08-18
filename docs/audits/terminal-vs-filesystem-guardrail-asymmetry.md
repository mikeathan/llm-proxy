---
status: complete
last_reviewed: 2026-08-20
---

# Audit: Terminal vs Filesystem Guardrail Asymmetry — `.sandbox` Readable via Terminal

## Symptom

The `workspace-health-test` automation run `20260820T161430Z_d9242235b4861237`
inspected and reported on the `.sandbox/` directory (44M, npm cache blobs under
`.sandbox/.npm/_cacache/`, `.sandbox/tmp/`), recommending cleanup of files inside it.
The earlier run `20260820T154509Z_7af85d8a9a44af34` had zero `.sandbox` references —
the regression was model-driven (the run went off the `du -sh *` template and ran
`du -sh .sandbox` / `find .sandbox/...` explicitly).

## Root Cause — tool-layer asymmetry (not a channel leak)

Two tools touch workspace files, with different guardrail coverage:

1. **Filesystem tool** (`read_file` / `list_directory` / `write_file`) already blocks
   **all hidden dot-dirs** via the `hasHidden` check in `filesystem.go`
   (`read_file(".sandbox/...")` was rejected).
2. **Terminal tool** (`execute_terminal_command`) had **no** path-operand block.
   `checkPathSecurity` only blocked absolute-paths-outside-the-jail and `..` traversal.
   Relative in-jail operands like `.sandbox` passed freely.

Assistant and automation share the **same** tools and guardrail engine
(`NewTerminalTools` in `registry.go`), so the gap was identical in both channels — it
surfaced in automation only because the health-audit task is a `du`/`find`/`cat`
workload that naturally enumerates the workspace. `.sandbox` is the sandbox runtime
HOME (`terminal.go` `workspaceEnvTemplates`), gitignored (`gitignore`), and **not**
named in any guardrail config — it was caught by the filesystem tool only incidentally
(because it starts with a dot).

## Fix

Added a targeted, shared-layer terminal block (not a blanket hidden-dir block, which
would break `git` and package managers):

- **`.sandbox` is hardcoded** in `checkBlockedPaths` (`terminal.go`, `sandboxRuntimeDirs`)
  as a code invariant — the sandbox runtime dir is not user-configurable, so no config
  knob is needed.
- **Sensitive files** are enforced via the **existing shared `blocked_filenames`** list
  (`filesystem.json`), which the terminal now inherits by reusing `FileSystem.BlockedFilenames`
  (threaded through `validateTerminal` → `ValidateTerminalCommand`). Default emits:
  `.env`, `.ssh`, `.pem`, `id_rsa`, `id_ed25519`, plus more credential/private-key names
  (`.git-credentials`, `.npmrc`, `.netrc`, `.pypirc`, `.htpasswd`, `.pgpass`, `.kubeconfig`,
  `.aws`, `.gnupg`, `credentials.json`, `id_dsa`, `id_ecdsa`, `.key`, `.p12`, `.pfx`).
- Both blocks return `"path access denied: ..."` so `isGuardrailSecurityBoundary`
  (`tool_exec.go`) classifies them as **silent policy denials** — no allow/deny prompt.
- The filesystem tool's `blocked_filenames` error was aligned to the same `"path access
  denied"` prefix for consistent silent-denial behavior.
- `config.json` was removed from `blocked_filenames` — it is legacy (the modern config
  document is `settings.yml`) and already protected by the hardcoded system-file list
  (`filesystem.go` `SystemConfigFilename`).

## Guardrail decision — why not a blanket hidden-dir block

The filesystem tool can afford a blunt "block any dot-dir" rule because it is a narrow
file API. The terminal runs arbitrary allowed commands (`git`, `npm`, `node`, `go`,
`cat`) that necessarily read/write dot-dirs (`.git/`, `.npmrc`, `.sandbox/.npm`,
`.sandbox/node_modules/.bin`). A blanket block would break those. Therefore the
terminal blocks a **curated sensitive set**, not every dot-dir.
