## Task: Network Audit

**ID:** `network-audit`
**Category:** network
**Summary:** Scans the networks you allowed, checks them for common weak settings, compares with the last scan and tells you what is new, changed or gone.
**Agent:** security-auditor
**Network:** lan
**Journal:** off
**Schedule:** 0 2 * * *
**Depth:** thorough
**Checks:** on
**Known vulnerabilities:** on

Scan the network and report what changed since the last scan: new open ports, services that disappeared, software versions that changed and weak settings the safe checks found. The scan facts are written by the system, not the model, so the report is comparable from one scan to the next. It runs on the schedule you choose when you create the automation (every night at 02:00 to start with); set it to manual to run it only when you press Run.

### Before the first run

- Settings → Network scan: choose the network that may be scanned. Only addresses inside it can be scanned.
- nmap must be installed on this machine. An audit does not run without it and the failure says so.

### What it scans

What is scanned and how (the networks, quick or thorough, the ports, and whether the safe checks run) is set on the automation, in the form and on its page, not in this text. The system applies those settings when the audit runs. By default it scans every network you allowed, thoroughly, with the checks on.

The checks look for SMBv1 and unsigned SMB, anonymous FTP, a VNC, Redis, MongoDB, Memcached or MQTT broker with no password, exposed NFS shares, Telnet and an open Docker control port, expired or weak certificates, weak SSH algorithms and host keys, and Remote Desktop without network-level authentication. They only read; nothing tries a password and nothing changes on a device.

The system also compares the software the scan recognised with a list of publicly known flaws and adds those findings to the report itself, after your assessment. You do not see them, so do not write about known vulnerabilities, and never say none were found.

### Extra instructions (optional)

Anything the assessment should take into account, in plain words, for example "the printer on .30 is expected to have port 631 open". Leave this empty if there is nothing to add.

### Execution Strategy

**Action:** Run `network_scan` once. The system applies this audit's settings whatever you pass, so pass `mode: "checks"` and nothing else.

Then write a short assessment (the system adds the observed facts, with every check's problem and fix, above it), following the extra instructions. Do not restate the problems the checks found. If nothing is new or changed, say so in one sentence.

### Result

**PASS** if the scan ran and the assessment covers what is new or changed (or states there are none). **FAIL** with the reason if the scan was refused or could not run.
