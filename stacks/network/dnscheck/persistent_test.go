package dnscheck

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func row(name string, outcome Outcome, overlap bool, minutesAfter int) Comparison {
	return Comparison{At: t0.Add(time.Duration(minutesAfter) * time.Minute), Name: name, Outcome: outcome, Overlap: overlap}
}

func TestPersistent_ADifferenceOnEveryCheckInARunIsOffered(t *testing.T) {
	log := []Comparison{
		row("a.example", Match, false, 0), row("a.example", Differ, false, 60), row("a.example", Differ, false, 120),
		row("b.example", Match, false, 0), row("b.example", Match, false, 60), row("b.example", Match, false, 120),
	}
	got := Persistent(log, t0.Add(121*time.Minute))
	if len(got.Names) != 1 || got.Names[0] != "a.example" || got.Count != 2 || !got.Since.Equal(t0.Add(60*time.Minute)) {
		t.Fatalf("got %+v", got)
	}
}

func TestPersistent_OneDifferenceIsNotEnough(t *testing.T) {
	log := []Comparison{row("a.example", Match, false, 0), row("a.example", Differ, false, 60)}
	if got := Persistent(log, t0.Add(61*time.Minute)); len(got.Names) != 0 {
		t.Fatalf("one differing check is a blip: %+v", got)
	}
}

// A match ends the run, a rotated answer (shared address) is not a difference, and an unreadable check neither extends nor ends it.
func TestPersistent_MatchEndsRunOverlapIsNotADifferenceAndCannotCompareIsNeutral(t *testing.T) {
	log := []Comparison{row("a.example", Differ, false, 0), row("a.example", Differ, false, 60), row("a.example", Match, false, 120)}
	if got := Persistent(log, t0.Add(121*time.Minute)); len(got.Names) != 0 {
		t.Errorf("a later match ends the run: %+v", got)
	}
	log = []Comparison{row("a.example", Differ, true, 0), row("a.example", Differ, true, 60)}
	if got := Persistent(log, t0.Add(61*time.Minute)); len(got.Names) != 0 {
		t.Errorf("answers that share an address are a rotation, not a difference: %+v", got)
	}
	log = []Comparison{row("a.example", Differ, false, 0), row("a.example", CannotCompare, false, 60), row("a.example", Differ, false, 120)}
	if got := Persistent(log, t0.Add(121*time.Minute)); got.Count != 2 {
		t.Errorf("a check that could not compare is skipped: %+v", got)
	}
}

// A difference nobody has re-checked for a day is old news, so a switched-off check does not keep a finding alive.
func TestPersistent_StaleDifferencesAreNotOffered(t *testing.T) {
	log := []Comparison{row("a.example", Differ, false, 0), row("a.example", Differ, false, 60)}
	if got := Persistent(log, t0.Add(60*time.Minute+Freshness+time.Minute)); len(got.Names) != 0 {
		t.Fatalf("stale: %+v", got)
	}
}

func TestPersistent_CountIsTheShortestRunAndNamesAreSorted(t *testing.T) {
	log := []Comparison{
		row("b.example", Differ, false, 0), row("b.example", Differ, false, 60), row("b.example", Differ, false, 120),
		row("a.example", Differ, false, 60), row("a.example", Differ, false, 120),
	}
	got := Persistent(log, t0.Add(121*time.Minute))
	if len(got.Names) != 2 || got.Names[0] != "a.example" || got.Count != 2 {
		t.Fatalf("got %+v", got)
	}
}
