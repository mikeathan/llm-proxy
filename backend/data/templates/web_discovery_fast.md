## Task: Fast Web Service Discovery
**ID:** `web-discovery-fast`
**Category:** Audit

Scan specifically for web applications and management interfaces, identifying software stacks and redirect behavior.

### Execution Strategy

#### Phase 1: Web Service Discovery
**Action:** Use `scan_local_network` with specific ports: [80,443,3000,4000,5000,8080,8443,9000].
*   Identifies open web and API ports across the target range.

#### Phase 2: Header Grabbing
**Action:** Use `fetch_url` on identified endpoints.
*   The tool handles following redirects and capturing header metadata automatically.

### Output Format
1. Summary of Web Services
2. Web Service Audit Table (URL, Server Header, Technology, Security Status)
3. Identification of Management Interfaces (Admin panels, IDEs, Model Servers)
4. Recommendations for Header Hardening
