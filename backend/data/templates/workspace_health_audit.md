## Task: Workspace Storage & Health Audit

**ID:** `workspace-health-audit`
**Category:** system

Monitor the physical health of the workspace environment and identify resource-heavy files or directories.

### Scope

Audit only non-hidden workspace content. Do **not** inspect, measure, or report on the `.sandbox/`
directory (the sandbox runtime dir) or any hidden dot-directory (e.g. `.git/`, `.npm/`, `.config/`).
Those are outside the audit scope; the shell glob `*` already excludes them.

### Execution Strategy

#### Phase 1: Resource Discovery

Run `df -h` and `uptime` to check system-level availability.

#### Phase 2: Storage Analysis

Run `du -sh *` to identify the largest directories and files within the workspace. The glob
`*` does not match hidden entries, so `.sandbox/` and other dot-directories are excluded automatically;
do not run `du -sh .sandbox` or otherwise probe hidden directories.

#### Phase 3: Cleanup Identification

Use `list_directory` to find temporary files or log backups that exceed 10MB. Only consider
non-hidden workspace entries; ignore `.sandbox/` and hidden dot-directories.

### Output Format

1. System status (disk free, uptime)
2. Largest directories (top 3 by size)
3. Temporary files identified (path and size)
