## Task: Workspace Storage & Health Audit

**ID:** `workspace-health-audit`
**Category:** system

Monitor the physical health of the workspace environment and identify resource-heavy files or directories.

### Execution Strategy

#### Phase 1: Resource Discovery

Run `df -h` and `uptime` to check system-level availability.

#### Phase 2: Storage Analysis

Run `du -sh *` to identify the largest directories and files within the workspace.

#### Phase 3: Cleanup Identification

Use `list_directory` to find temporary files or log backups that exceed 10MB.

### Output Format

1. System status (disk free, uptime)
2. Largest directories (top 3 by size)
3. Temporary files identified (path and size)
