---
id: ADR-0027
title: Installed packages are judged against Canonical's per-release USN OVAL file, with Pro-only fixes left out
status: accepted
date: 2026-11-04
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0027: Installed packages are judged against Canonical's per-release USN OVAL file, with Pro-only fixes left out

## Context
The owner approved (2026-11-04, plan `network-attempts-integrity-timeline` P6) a check that says which installed packages are behind a published security fix. NVD matching by version is the wrong tool for a distribution: Ubuntu backports fixes without raising the upstream version, which is the false "high" the ledger recorded for Samba (2026-10-28). Vertex is Ubuntu noble (`VERSION_CODENAME`, captured 2026-11-04), `/var/lib/dpkg/status` is `-rw-r--r-- root` (2.1 MB, 1,999 packages), and the login message already says some fixes are offered only with Ubuntu Pro.

## Decision
1. **Source: Canonical's USN OVAL file for this computer's release**, `https://security-metadata.canonical.com/oval/com.ubuntu.<codename>.usn.oval.xml.bz2` (noble, measured 2026-11-04: 1,292,296 bytes compressed, 37.7 MB of XML, 1,580 patch definitions). The CVE OVAL (11.2 MB) and the notices web API (rate limited) are not used. The host and the path are fixed in code; the codename comes from `/etc/os-release` and must be `ID=ubuntu` and a plain lower-case word, so no other address can be built.
2. **An update is one click, like the vulnerability data's download** (a signed-in action through the guarded opener, so the Guardrails internet switch and the blocked-domain list apply). Deleting the data is a step-up route (it removes what every audit compares with). There is no hand import: the file is small, and a hand-made file could silence the check.
3. **What is kept is ids and versions only.** For each binary package, the version that fixed a flaw and the notice; for each notice, its date, worst severity and flaw ids. A notice's prose, a package name or version that is not plain text, and a flaw id that does not have the shape of one never enter the index. A reply that holds no usable notice never replaces data already held.
4. **Pro-only fixes are left out.** 268 of 1,384 fix versions in the real noble file carry `esm` (for example `1.7.17-1ubuntu0.1~esm2`); only a subscription installs those, so counting them would flag packages the owner cannot update. Kernel live-patch notices (`LSN-`) are left out too. The finding says that Pro-only updates are not counted.
5. **Matching uses the dpkg ordering** (`ubuntuoval.Compare`, tested against known orderings and 20,000 random triples for antisymmetry and transitivity). A package is behind when its installed version is older than a fix's version and that notice is at least 48 hours old (a fix takes about a day to reach the mirrors and automatic updates).
6. **One finding for the computer, `host_packages_vulnerable`**, with the number of packages and flaws and up to ten package names; high when one of the flaws is on CISA's exploited list (the list the program already imports, ADR-0011), medium for a critical or high flaw, low otherwise. A package list that cannot be read keeps the earlier finding and counts as an unreadable place; a computer with no data (or data for another release) says nothing about its packages.

## Consequences
- The data's licence is not stated by Canonical (a 2023 informal reply says it is shared so that others use it). The card says so and attributes it; the file is fetched once on request, kept locally and never sent anywhere. The owner approved on that basis.
- A package from another source than Ubuntu's archive is judged only if the notices name its binary package; one that is not named is not judged.
- A fix that exists only for Ubuntu Pro leaves the package unreported, which can hide a flaw the owner may want to know about. The card and the finding say so.
- The first update has to be pressed by the owner; until then the audit says nothing about packages.
