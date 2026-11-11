---
id: ADR-0043
title: The traffic look reads a TLS server name and names the holder of an address on this computer
status: proposed
date: 2026-11-11
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0043: The traffic look reads a TLS server name and names the holder of an address on this computer

## Context

The Traffic tab (ADR-0040) shows where this computer's traffic went by address, port and, when a DNS answer gave one, a name. The owner asked on 2026-11-11 for
more about the destinations: where they are, who runs them, and a nice way to see it. Two gaps were measured in the code, not guessed: a destination with no DNS
answer in the window (a cached lookup, DNS over HTTPS, a service reached by a stored address) is a bare address, and nothing says whose network any internet address
belongs to. ADR-0040 left the first open on purpose ("a TLS server name is not read ... its own amendment") and Constitution I.13 forbade it.

Sources for the second were checked on 2026-11-11 (downloaded and read, not remembered): IPtoASN (public domain, no key, hourly; `ip2asn-v4.tsv.gz` 8.7 MB and
`ip2asn-v6.tsv.gz` 2.3 MB, 579,453 usable ranges, each with the AS number, a two-letter country and the holder's name), DB-IP Lite (country only, CC BY 4.0, monthly).
A per-address online lookup (ipinfo, RDAP, a geolocation API) would send every destination this computer talks to, to a third party.

## Decision

We will do both, and keep every lookup on this computer.

- **A ClientHello keeps more of its frame.** The host filter (`capture/filter_host.go`) finds the TCP payload from the data offset and, when it starts like a ClientHello
  (`16 03 .. .. .. 01`), returns 768 bytes of the frame instead of the headers and ports. It does this with classic BPF scratch memory and a frame-length load, which
  the unit interpreter models and a real kernel ran (`host_linux_test.go`, in a container with `CAP_NET_RAW`). A load past the end of a frame ends a classic BPF program
  with a drop, so the filter first checks, with the frame-length load, that six payload bytes exist; an acknowledgement is therefore still an ordinary frame and not a hole
  in the totals (a test on the interpreter and one on a real kernel both pin it). A hello is never sampled away, like a DNS message.
- **The service reads one field.** `hosttraffic/tls.go` walks the record header, the fixed hello fields, the three length-prefixed vectors and at most 64 extensions, with a
  bound before every read, and returns the `server_name` host name vetted by the DNS-name rule of I.8. A name of the conversation's own hello beats the name of the address
  from DNS (one address serves many sites), and the view says which it was (`name_from`: `dns` or `tls`). A hello that came in names nothing.
- **A table of address owners, downloaded on a press.** `core/netaudit/ipowners` parses the two IPtoASN files into sorted ranges (IPv4 as numbers, IPv6 as 16 bytes, holder
  names stored once), answers by binary search, drops overlapping ranges so the search stays sound, and refuses an empty file. The download goes through `NetworkTools.Open`
  to two fixed addresses, is a step-up action (`POST /admin/api/netaudit/destination-data/download`), is bounded in size and rows, and keeps the old table when it fails. The
  table is stored as the two compressed files in one factory-reset file; loading it takes about 0.45 s (measured on the real data, `IPOWNERS_LIVE=1`).
- **Only internet addresses are looked up**, once per address per window. The result is two ranked lists (by holder, by country) over the conversations the table kept, plus an
  owner line on each destination and conversation. The holder's name is made plain text and shown only on this view.
- **No map.** The country says who an address is registered to, not where the machine is; a map would draw false precision (a CDN shows one country) and add a dependency. Ranked
  bars by holder and by country answer "where does it go" without it.

## Consequences

- A destination that DNS did not name usually gets a name, and every internet destination can say whose network it is on, with nothing sent anywhere to find out.
- The helper keeps up to 768 bytes of a ClientHello frame: handshake metadata with no application data, but more than "headers and ports". This is a constitutional amendment
  (I.13) and the owner's approval of it is the merge. The cap is the output cap already there (8 MiB, 60,000 frames); a host making hundreds of handshakes a second reaches it
  sooner, and the window says it stopped at the limit.
- The helper's capture program doubles its sampling when it keeps more than 800 frames a second; hellos are kept whatever the rate, so a host with that many handshakes a second
  samples harder than it needs to. Not measured; noted in the learnings ledger.
- A hello with its name past the first 768 bytes (Chrome puts a large key-share first in some orders) gives no name and the address stays as it was. QUIC and Encrypted Client
  Hello are not read; a hello over IPv6 with extension headers is treated as an ordinary frame.
- The table is 11 MB on disk and about 20 MB in memory once loaded, and is refreshed by hand. It is public data and carries no operator secret.

## Alternatives considered

- **A per-address online lookup (ipinfo, ip-api, RDAP).** Sends every destination to a third party; ruled out by I.2's spirit and by the point of the audit.
- **MaxMind GeoLite2.** Needs an account, a licence key and an end-user agreement; IPtoASN needs none and gives the holder as well as the country.
- **DB-IP Lite alongside IPtoASN.** Country only, which IPtoASN already gives; one source is simpler and needs no attribution.
- **A world map.** False precision for CDN-served traffic, a new dependency, and the question "who runs it" is better answered by a name than a dot.
- **Reverse DNS on each address.** Sends the address to the resolver and gives a host name, which the DNS answers and hellos already give better.
- **Keep the whole frame for every TCP segment.** Payload of every connection; far past what the owner approved. Only a ClientHello keeps more.
- **Read the hello in the helper.** The helper parses nothing (I.11, I.13): the service reads it.

## Links

- Plan: `docs/PLANS/cross-cutting/network-clickable-findings-traffic-owners.md`
- Constitution I.13 (amended 2026-11-11); ADR-0040; SPEC-003 changelog 2.60.
- Code: `backend/internal/platform/privhelper/capture/filter_host.go`, `backend/internal/core/netaudit/hosttraffic/{tls,owners}.go`, `backend/internal/core/netaudit/ipowners/`.
