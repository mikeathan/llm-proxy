## Task: Network Reconnaissance (Non-Privileged)

**ID:** `network-recon-unprivileged`
**Category:** network

Perform a multi-phase, optimized network scan to identify active hosts and their exposed services without requiring root privileges.

### Scan Target

- Scan the **local subnet of this host** (derive it from the host's own IP via the `get_network_info` tool, or scan the detected gateway's subnet, e.g. `192.168.x.0/24`).
- Do NOT scan external/cloud ranges or the metadata IP `169.254.169.254`.
- `network_scan` only scans addresses the operator has allowed (Settings → Network scan). If it refuses a target, say so in the report and stop; do not try other addresses.

### Execution Strategy

#### Phase 1: Rapid Host Discovery
**Action:** Use `network_scan` with `mode: "discover"` and the local subnet as the target.
*   Finds which addresses answer.

#### Phase 2: Targeted Service Audit
**Action:** Use `network_scan` with `mode: "services"` for the IPs found in Phase 1, all in one call.
*   Finds open ports and identifies the service and version on each. It needs nmap; if the tool says nmap is required, report that and use `mode: "ports"` instead.

#### Phase 3: Evidence Verification
For each open port reported, **confirm it with a second probe** (re-run `network_scan` with `mode: "ports"` and `ports` set to that one port on that IP) and record the evidence. Do not list a port as open without confirmation.

### Output Format
Generate a Markdown report with sections for:
1. Summary of Findings
2. Open Ports & Services Table (Port, Protocol, Service, Version, Risk, **Evidence: confirmed by re-probe yes/no**)
3. Technical Analysis
4. Hardening Recommendations (one actionable step per finding)

### Result

**PASS** if the report includes a Summary, an Open Ports table, Technical Analysis, and Hardening Recommendations — and every listed port has confirmation evidence. If no hosts were found on the subnet, **PASS** only if the report explicitly states "no active hosts found on <subnet>". Otherwise **FAIL** and state the reason.
