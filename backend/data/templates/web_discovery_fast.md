## Task: Fast Web Service Discovery
**ID:** `web-discovery-fast`
**Category:** Audit

Scan specifically for web applications and management interfaces, identifying software stacks and redirect behavior.

### Execution Strategy

#### Phase 1: Port Discovery (Web Focused)
**Command:** `nmap -sT -Pn -p 80,443,3000,4000,5000,8000,8080,8081,8443,9000,9001,9002 -T4 {{target}}`

#### Phase 2: Banner Grabbing
**Command:** `curl -I -s --connect-timeout 5 http://[IP]:[PORT]`
*   Check for `Server` headers, `X-Powered-By`, and `Location` redirects.

### Output Format
1. Summary of Web Services
2. Web Service Audit Table (URL, Server Header, Technology, Security Status)
3. Identification of Management Interfaces (Admin panels, IDEs, Model Servers)
4. Recommendations for Header Hardening
