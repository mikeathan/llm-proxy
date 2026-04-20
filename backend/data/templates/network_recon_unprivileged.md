## Task: Network Reconnaissance (Non-Privileged)
**ID:** `network-recon-unprivileged`
**Category:** Reconnaissance

Perform a multi-phase, optimized network scan to identify active hosts and their exposed services without requiring root privileges.

### Execution Strategy

#### Phase 1: Rapid Host Discovery
**Command:** `nmap -sn -PS22,80,443 -T4 {{target_range}}`
*   `-sn`: Ping scan (no port scan).
*   `-PS22,80,443`: TCP SYN Ping on common ports (bypasses ICMP blocks).

#### Phase 2: Targeted Port Scan
**Command:** `nmap -sT -sV -p [PORTS_FROM_PHASE_1] [IPs_FROM_PHASE_1] --version-intensity 3`
*   `-sT`: TCP Connect scan (Unprivileged).
*   `-sV`: Version detection.

### Output Format
Generate a Markdown report with sections for:
1. Summary of Findings
2. Open Ports & Services Table (Port, Protocol, Service, Version, Risk)
3. Technical Analysis
4. Hardening Recommendations
