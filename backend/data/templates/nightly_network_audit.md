## Task: Nightly Network Audit

**ID:** `nightly-network-audit`
**Category:** network

Scan the network once a night and report what changed since the last scan: new open ports, services that disappeared and software versions that changed. The scan facts are written by the system, not the model, so the report is comparable night to night.

### Set up (once)

- Settings → Network scan: choose the network that may be scanned. Only addresses inside it can be scanned.
- Create the automation with the **Security auditor** agent, network access **Local network only**, the journal off and recording off (a recording keeps every scan result).
- nmap must be installed on this machine. A nightly audit does not run without it and the failure says so.
- Edit the TARGETS line below to the network (or addresses) to audit. Use the same network you allowed above.

### Targets

TARGETS: 192.168.1.0/24

### Execution Strategy

**Action:** Run `network_scan` once with `targets` set to the TARGETS above, `mode: "services"` and `thorough: true`.

Then write your assessment (the system adds the observed facts above it): the new or changed findings ranked by risk, each with the one step that would fix it. If nothing is new or changed, say so in one sentence.

### Result

**PASS** if the scan ran and the assessment covers every new or changed finding (or states there are none). **FAIL** with the reason if the scan was refused or could not run.
