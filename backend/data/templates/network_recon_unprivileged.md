## Task: Network Reconnaissance (Non-Privileged)

**ID:** `network-recon-unprivileged`
**Category:** network

Perform a multi-phase, optimized network scan to identify active hosts and their exposed services without requiring root privileges.

### Scan Target

- Scan the **local subnet of this host** (derive it from the host's own IP via the `get_network_info` tool, or scan the detected gateway's subnet, e.g. `192.168.x.0/24`).
- Do NOT scan external/cloud ranges or the metadata IP `169.254.169.254`.

### Execution Strategy

#### Phase 1: Rapid Host Discovery
**Action:** Use `scan_local_network` with `mode: "fast"` on the local subnet.
*   Performs a rapid sweep of common discovery ports to identify active IPs.

#### Phase 2: Targeted Service Audit
**Action:** Use `scan_local_network` with `mode: "deep"` for specific IPs found in Phase 1.
*   Performs service identification and banner fingerprinting.

#### Phase 3: Evidence Verification
For each open port reported, **confirm it with a second probe** (re-run a targeted deep scan on that IP:port, or probe the port directly) and record the evidence. Do not list a port as open without confirmation.

### Output Format
Generate a Markdown report with sections for:
1. Summary of Findings
2. Open Ports & Services Table (Port, Protocol, Service, Version, Risk, **Evidence: confirmed by re-probe yes/no**)
3. Technical Analysis
4. Hardening Recommendations (one actionable step per finding)

### Result

**PASS** if the report includes a Summary, an Open Ports table, Technical Analysis, and Hardening Recommendations — and every listed port has confirmation evidence. If no hosts were found on the subnet, **PASS** only if the report explicitly states "no active hosts found on <subnet>". Otherwise **FAIL** and state the reason.
