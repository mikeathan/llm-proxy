## Task: Workspace Storage & Health Audit

**ID:** `workspace-health-audit`
**Category:** Maintenance

Monitor the physical health of the workspace environment and identify resource-heavy files or directories.

## Execution Strategy

#### Phase 1: Resource Discovery

**Action:** Use execute_terminal_command with df -h and uptime to check system-level availability.

#### Phase 2: Storage Analysis

**Action:** Use du -sh * to identify the largest directories and files within the authorized jail.

#### Phase 3: Cleanup Identification

**Action:** Use list_directory to find temporary files or log backups that exceed 10MB.
