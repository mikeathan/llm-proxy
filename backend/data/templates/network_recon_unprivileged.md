## Task: Network Reconnaissance (Non-Privileged)
**ID:** `network-recon-unprivileged`
**Category:** network

Perform a multi-phase, optimized network scan to identify active hosts and their exposed services without requiring root privileges.

### Execution Strategy

#### Phase 1: Rapid Host Discovery
**Action:** Use `scan_local_network` with `mode: "fast"`.
*   Performs a rapid sweep of common discovery ports to identify active IPs.

#### Phase 2: Targeted Service Audit
**Action:** Use `scan_local_network` with `mode: "deep"` for specific IPs found in Phase 1.
*   Performs service identification and banner fingerprinting.

### Output Format
Generate a Markdown report with sections for:
1. Summary of Findings
2. Open Ports & Services Table (Port, Protocol, Service, Version, Risk)
3. Technical Analysis
4. Hardening Recommendations
