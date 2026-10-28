package netaudit

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"llm-proxy/models"
)

// Acceptance lengths. An acceptance always expires, so a risk someone once accepted does not stay hidden for ever.
const (
	DefaultAckDays = 90
	MaxAckDays     = 365
	// ackStaleAfter drops an acceptance whose finding no scan has shown for this long.
	ackStaleAfter = 30 * 24 * time.Hour
	maxNoteLen    = 200
)

// ErrBadAck means an acceptance request is not valid.
var ErrBadAck = errors.New("invalid acknowledgement")

// NewAck accepts a finding for days days (DefaultAckDays when zero). The accepted version is the finding's version at
// this moment: if it changes, the acceptance stops applying.
func NewAck(f Finding, note string, days int, now time.Time) (models.ScanAck, error) {
	if days == 0 {
		days = DefaultAckDays
	}
	if days < 1 || days > MaxAckDays {
		return models.ScanAck{}, fmt.Errorf("%w: days must be between 1 and %d", ErrBadAck, MaxAckDays)
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > maxNoteLen {
		return models.ScanAck{}, fmt.Errorf("%w: the note is longer than %d characters", ErrBadAck, maxNoteLen)
	}
	return models.ScanAck{
		Key: f.Key(), AcceptedVersion: f.Version, Note: note,
		AcceptedAt: now, ExpiresAt: now.AddDate(0, 0, days), LastSeen: now,
		AcceptedCVEs: slices.Clone(f.CVEs), AcceptedExploited: slices.Clone(f.Exploited),
	}, nil
}

// AckState says whether an acceptance applies to a finding right now.
type AckState string

const (
	AckActive         AckState = "active"
	AckExpired        AckState = "expired"
	AckVersionChanged AckState = "version_changed"
	AckNewVulns       AckState = "new_vulns"
)

// StateFor reports whether the acceptance still covers the finding. The finding must be the one it names.
func StateFor(a models.ScanAck, f Finding, now time.Time) AckState {
	switch {
	case !now.Before(a.ExpiresAt):
		return AckExpired
	case a.AcceptedVersion != f.Version:
		return AckVersionChanged
	case hasNewID(f.CVEs, a.AcceptedCVEs) || hasNewID(f.Exploited, a.AcceptedExploited):
		return AckNewVulns
	}
	return AckActive
}

// hasNewID reports whether current holds an id that accepted does not. A smaller set is not new: fewer flaws never
// bring an accepted finding back.
func hasNewID(current, accepted []string) bool {
	for _, id := range current {
		if !slices.Contains(accepted, id) {
			return true
		}
	}
	return false
}

// AckResult is the findings split by the acceptances that apply, with the acceptances brought up to date.
type AckResult struct {
	// Visible are the findings to report: not accepted, or the acceptance expired or no longer matches.
	Visible []Finding
	// Accepted are the findings an active acceptance covers.
	Accepted []Finding
	// Acks is the acceptance list to keep: LastSeen refreshed for those whose finding showed up, and the ones no scan has
	// shown for a long time removed.
	Acks []models.ScanAck
}

// ApplyAcks splits the findings into the ones to report and the ones an operator accepted. It is pure.
func ApplyAcks(findings []Finding, acks []models.ScanAck, now time.Time) AckResult {
	byKey := make(map[string]int, len(acks))
	kept := slices.Clone(acks)
	for i, a := range kept {
		byKey[a.Key] = i
	}
	var res AckResult
	for _, f := range findings {
		i, ok := byKey[f.Key()]
		if !ok {
			res.Visible = append(res.Visible, f)
			continue
		}
		kept[i].LastSeen = now
		if StateFor(kept[i], f, now) == AckActive {
			res.Accepted = append(res.Accepted, f)
		} else {
			res.Visible = append(res.Visible, f)
		}
	}
	for _, a := range kept {
		if !isStale(a, now) {
			res.Acks = append(res.Acks, a)
		}
	}
	return res
}

func isStale(a models.ScanAck, now time.Time) bool {
	seen := a.LastSeen
	if seen.IsZero() {
		seen = a.AcceptedAt
	}
	return now.Sub(seen) > ackStaleAfter
}

// UpsertAck adds an acceptance, replacing any earlier one for the same finding.
func UpsertAck(acks []models.ScanAck, ack models.ScanAck) []models.ScanAck {
	out := make([]models.ScanAck, 0, len(acks)+1)
	for _, a := range acks {
		if a.Key != ack.Key {
			out = append(out, a)
		}
	}
	return append(out, ack)
}

// RemoveAck drops the acceptance for a finding; ok is false when there was none.
func RemoveAck(acks []models.ScanAck, key string) ([]models.ScanAck, bool) {
	out := make([]models.ScanAck, 0, len(acks))
	for _, a := range acks {
		if a.Key != key {
			out = append(out, a)
		}
	}
	return out, len(out) != len(acks)
}

// HideAccepted returns the scan result as the model should see it: the open ports an active acceptance covers are left
// out of the hosts and the findings, and the number left out is returned. The input is not changed.
func HideAccepted(res Result, acks []models.ScanAck, now time.Time) (Result, int) {
	if len(acks) == 0 {
		return res, 0
	}
	accepted := map[string]bool{}
	for _, f := range ApplyAcks(res.Findings, acks, now).Accepted {
		accepted[f.Key()] = true
	}
	if len(accepted) == 0 {
		return res, 0
	}
	out := res
	out.Hosts = make([]Host, 0, len(res.Hosts))
	hidden := 0
	for _, h := range res.Hosts {
		kept := h
		kept.Ports = nil
		for _, p := range h.Ports {
			key := Finding{Device: h.Addr.String(), Port: p.Number, Proto: p.Proto, Check: CheckOpenPort}.Key()
			if p.State == "open" && accepted[key] {
				hidden++
				continue
			}
			kept.Ports = append(kept.Ports, p)
		}
		out.Hosts = append(out.Hosts, kept)
	}
	out.Findings = FindingsOf(out.Hosts)
	return out, hidden
}
