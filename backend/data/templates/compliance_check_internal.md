## Task: Compliance & High-Risk Port Audit

**ID:** `compliance-check-internal`
**Category:** network

Audit internal hosts for the presence of legacy or high-risk services that should not be exposed.

### Scan Target

- Scan the **local subnet of this host** (derive it from the host's own IP via the `get_network_info` tool, or scan the detected gateway's subnet, e.g. `192.168.x.0/24`).
- Do NOT scan external/cloud ranges or the metadata IP `169.254.169.254`.

### Execution Strategy

#### Action: High-Risk Service Audit
**Action:** Use `scan_local_network` with specific ports: [21,23,111,445,2049,3389,5900] on the local subnet.
*   **Port 21/23**: FTP/Telnet (Unencrypted Admin)
*   **Port 445**: SMB (File Sharing)
*   **Port 111/2049**: NFS (Network File System)
*   **Port 3389/5900**: Remote Desktop (RDP/VNC)

#### Evidence Verification
For every open high-risk port reported, **confirm it with a second probe** (re-run a targeted deep scan on that IP:port, or probe the port directly) and record the evidence. Do not list a port as open without confirmation.

### Analysis Goals
*   Verify if exposure is justified by business need.
*   Check for cleartext credential transmission (Telnet/FTP).
*   Flag SMB exposure as a high-risk lateral movement vector.

### Output Format
1. High-Risk Findings Summary
2. Policy Violation Table (Port, Protocol, Service, Potential Impact, **Evidence: confirmed by re-probe yes/no**)
3. Immediate Remediation Steps (Shutdown, Firewall, or Encrypted alternatives — one actionable step per finding)

### Result

**PASS** if the report includes a Summary, a Policy Violation Table where every listed port has confirmation evidence, and Immediate Remediation Steps. If no high-risk ports were found open, **PASS** only if the report explicitly states "no high-risk services found on <subnet>". Otherwise **FAIL** and state the reason.
