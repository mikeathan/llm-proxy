## Task: Fast Web Service Discovery

**ID:** `web-discovery-fast`
**Category:** network

Scan specifically for web applications and management interfaces, identifying software stacks and redirect behavior.

### Scan Target

- Scan the **local subnet of this host** (derive it from the host's own IP via the `get_network_info` tool, or scan the detected gateway's subnet, e.g. `192.168.x.0/24`).
- Do NOT scan external/cloud ranges or the metadata IP `169.254.169.254`.

### Execution Strategy

#### Phase 1: Web Service Discovery
**Action:** Use `scan_local_network` with specific ports: [80,443,3000,4000,5000,8080,8443,9000] on the local subnet.
*   Identifies open web and API ports across the target range.

#### Phase 2: Header Grabbing
**Action:** Use `fetch_url` on identified endpoints.
*   The tool handles following redirects and capturing header metadata automatically.

#### Phase 3: Evidence Verification
For every endpoint reported, confirm the HTTP status and `Server` header via `fetch_url` and record the actual response. Do not report an endpoint without a captured response.

### Output Format
1. Summary of Web Services
2. Web Service Audit Table (URL, Server Header, Technology, Security Status, **HTTP Status: captured yes/no**)
3. Identification of Management Interfaces (Admin panels, IDEs, Model Servers)
4. Recommendations for Header Hardening (one actionable step per finding)

### Result

**PASS** if the report includes a Summary, an Audit Table where every listed endpoint has a captured HTTP status + header, management-interface identification, and hardening recommendations. If no web endpoints were found, **PASS** only if the report explicitly states "no web services found on <subnet>". Otherwise **FAIL** and state the reason.
