## Task: Compliance & High-Risk Port Audit
**ID:** `compliance-check-internal`
**Category:** Security

Audit internal hosts for the presence of legacy or high-risk services that should not be exposed.

### Execution Strategy

#### Command: High-Risk Scan
**Command:** `nmap -sT -Pn -p 21,23,445,3389,5900,111,2049 -T4 --open {{target}}`
*   **Port 21**: FTP (Unencrypted)
*   **Port 23**: Telnet (Unencrypted Admin)
*   **Port 445**: SMB (File Sharing)
*   **Port 3389/5900**: Remote Desktop (RDP/VNC)
*   **Port 111/2049**: NFS (Network File System)

### Analysis Goals
*   Verify if exposure is justified by business need.
*   Check for cleartext credential transmission (Telnet/FTP).
*   Flag SMB exposure as a high-risk lateral movement vector.

### Output Format
1. High-Risk Findings Summary
2. Policy Violation Table (Port, Protocol, Service, Potential Impact)
3. Immediate Remediation Steps (Shutdown, Firewall, or Encrypted alternatives)
