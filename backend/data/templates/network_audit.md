## Task: Network Audit

**ID:** `network-audit`
**Category:** network
**Summary:** Scans the networks you allowed, checks them for common weak settings, compares with the last scan and tells you what is new, changed or gone.
**Agent:** security-auditor
**Network:** lan
**Journal:** off
**Schedule:** 0 2 * * *

Scan the network and report what changed since the last scan: new open ports, services that disappeared and software versions that changed. The scan facts are written by the system, not the model, so the report is comparable from one scan to the next. It runs on the schedule you choose when you create the automation (every night at 02:00 to start with); set it to manual to run it only when you press Run.

### Before the first run

- Settings → Network scan: choose the network that may be scanned. Only addresses inside it can be scanned.
- nmap must be installed on this machine. An audit does not run without it and the failure says so.

### What to scan

Edit these four lines to change the scan; leave them as they are for the usual audit.

TARGETS: all allowed
DEPTH: thorough
PORTS: default
CHECKS: on

- TARGETS: `all allowed` scans every network you allowed in Settings. Or list addresses, ranges or device names, separated by commas (for example `192.168.1.0/24, 192.168.1.5`). Anything outside the allowed networks is refused.
- DEPTH: `thorough` takes a couple of minutes and looks harder; `quick` takes seconds and checks the common ports only.
- CHECKS: `on` also runs a short fixed list of safe checks on the services it finds: SMBv1 and unsigned SMB, anonymous FTP, a VNC or Redis with no password, exposed NFS shares, expired certificates, weak SSH algorithms and Remote Desktop without network-level authentication. They only read; nothing tries a password and nothing changes on a device. `off` lists open ports and services only.
- PORTS: `default` uses the common ports. Or list them, for example `22,80,443,8000-8100`. To check only for risky remote-access and file-sharing services, use `21,23,111,445,2049,3389,5900`.

### Extra instructions (optional)

Anything the assessment should take into account, in plain words, for example "the printer on .50.30 is expected to have port 631 open". Leave this empty if there is nothing to add.

### Execution Strategy

**Action:** Run `network_scan` once with `mode: "checks"` when CHECKS is `on` and `mode: "services"` when it is `off`. Leave `targets` out when TARGETS says `all allowed`, otherwise pass the list. Set `thorough` to true for `thorough` and false for `quick`. Pass `ports` only when PORTS lists some.

Then write your assessment (the system adds the observed facts above it), following the extra instructions: the new or changed findings ranked by risk, each with the one step that would fix it. If nothing is new or changed, say so in one sentence.

### Result

**PASS** if the scan ran and the assessment covers every new or changed finding (or states there are none). **FAIL** with the reason if the scan was refused or could not run.
