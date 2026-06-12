## Task: Compliance & High-Risk Port Audit
**ID:** `compliance-check-internal`
**Category:** network

Audit internal hosts for the presence of legacy or high-risk services that should not be exposed.

### Execution Strategy

#### Action: High-Risk Service Audit
**Action:** Use `scan_local_network` with specific ports: [21,23,111,445,2049,3389,5900].
*   **Port 21/23**: FTP/Telnet (Unencrypted Admin)
*   **Port 445**: SMB (File Sharing)
*   **Port 111/2049**: NFS (Network File System)
*   **Port 3389/5900**: Remote Desktop (RDP/VNC)

### Analysis Goals
*   Verify if exposure is justified by business need.
*   Check for cleartext credential transmission (Telnet/FTP).
*   Flag SMB exposure as a high-risk lateral movement vector.

### Output Format
1. High-Risk Findings Summary
2. Policy Violation Table (Port, Protocol, Service, Potential Impact)
3. Immediate Remediation Steps (Shutdown, Firewall, or Encrypted alternatives)
