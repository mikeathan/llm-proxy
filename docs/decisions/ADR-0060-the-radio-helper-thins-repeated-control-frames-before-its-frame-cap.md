---
id: ADR-0060
title: The radio helper thins repeated control frames before its frame cap
status: proposed
date: 2026-11-18
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0060: The radio helper thins repeated control frames before its frame cap

## Context
A `radio_listen` window keeps at most 4,000 frames (`wire.RadioMaxFrames`). On vertex's own 5 GHz channel (60) the window filled that cap in
4.4 s and 3.7 s of its 20 s (the Listening page's windows, 2026-11-18), so the channel most of the owner's devices use got about 4 s of
listening per 5-minute cycle, and a dozing phone was heard only every 5 to 30 minutes (track of `4ded1379`). The 2026-11-13 capture on that
channel (`docs/learnings/network-audit.md`) shows why: about 1,290 frames a second, of which about 105k CTS, 85k RTS and 85k block-ack in
240 s, against 9.4k data frames. CTS and ACK name only a receiver, so the stack's parser gives them no sender and the recorder ignores them
(`stacks/radio/frame.go`, `recorder.go`). RTS and block-ack name a sender, and on 5 GHz they are often how a Wi-Fi 6 phone is heard at all
(the adapter cannot decode its data), but they repeat the same few senders hundreds of times a second; the stack uses one mean signal per
device per window, so the repeats add nothing.

Constitution I.16 says the helper "parses nothing" beyond the two bytes that tell a beacon. Dropping CTS and ACK needs only the frame control
byte (already read). Thinning repeats needs the sender's 6-byte address, which is reading a field: an amendment of I.16.

## Decision
The helper's `radio_listen` program (`capture.RunRadio`) passes every frame through `wire.RadioThinner` before the cap counts it:
- a control frame of subtype CTS or ACK is skipped;
- of any other control frame, the first 16 of each sender in the window are kept and the rest skipped (`RadioControlPerSender`);
- every management and data frame, and any frame the thinner cannot read, passes untouched;
- the sender table is bounded (`RadioThinSenders`, 1,024; a sender past it passes as before), lives for one window, and is never written or
  returned; the trailer carries the count left out (`Skipped`, bytes 24 to 27, which an older helper writes as 0).

I.16's "Headers only" item is amended to say so (proposed; in force when the owner merges the pull request that carries it). The owner chose
this option on 2026-11-18 over skipping CTS/ACK alone and over a larger cap for short frames.

## Consequences
- Channel 60's window should run its full 20 s instead of about 4 s: about 5 times the listening on the owner's channel at no extra frames.
  **Not yet measured on vertex**; the check is the Listening page's windows after deploy (no "Busy channel" badge on channel 60, "N repeats
  left out" instead).
- The stack sees fewer control frames per device: per-device control-frame counts (the 2026-11-13 idea of counting control frames when data
  undercounts) are now capped at 16 a window; nothing in the stack used them as totals.
- The helper now reads one address field of control frames. It is attacker-controlled bytes in a privileged process, so the read is a fixed
  6-byte copy at a bounds-checked offset into a map key, with no other parsing, tested with short and malformed frames.
- An older helper with a newer stack, or the reverse, still works: the trailer field is new reserved bytes.

## Alternatives considered
- **Skip CTS and ACK only.** No amendment, but about 40% of the frames, so the cap filled in about 5 to 6 s instead of 4.4 s.
- **A larger cap for 64-byte frames.** The full window, but about 5 times more frames to parse and the stack's hourly frame budget
  (`framesPerHour`, ADR-0056) raised; CPU on vertex not measured.
- **A second adapter parked on channel 60.** The better design for continuous listening (the 2026-11-13 measurement: one adapter per band),
  but new hardware; left for later by the owner.
- **A kernel BPF filter.** Would do the CTS/ACK part in the kernel, but not the per-sender thinning, and adds a second mechanism to review.

## Links
- Constitution I.16 (amended), I.17; ADR-0052, ADR-0053, ADR-0056
- `backend/internal/platform/privhelper/wire/radio.go` (`RadioThinner`), `backend/internal/platform/privhelper/capture/radio.go`
- `stacks/radio/window.go` (`Trailer.Skipped`, `WindowSummary.Skipped`)
- Handoff `docs/PLANS/cross-cutting/radio-filters-mobile-and-radar-movement-handoff-2026-11-18.md`
