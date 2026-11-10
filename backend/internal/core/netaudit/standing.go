package netaudit

import (
	"slices"
	"time"

	"llm-proxy/models"
)

// The standing answers "are we getting better?" from the finding history alone (ADR-0025: no history of its own). It is plain
// counts with a sentence on screen saying where each comes from, not one score.
const (
	TrendBetter  = "better"
	TrendWorse   = "worse"
	TrendSame    = "same"
	TrendUnknown = "unknown"

	standingWeek = 7 * 24 * time.Hour
	// standingEnough is how much history two whole weeks need; with less there is no comparison.
	standingEnough = 2 * standingWeek
	// OverMonthDays is the age past which an open problem counts as long-standing.
	OverMonthDays = 30
)

// StandingInput is what the standing is built from, already read.
type StandingInput struct {
	Now      time.Time
	Baseline models.ScanBaseline
	Acks     []models.ScanAck
	Life     []models.FindingLife
}

// WeekCounts are the problems that appeared and went away in one week.
type WeekCounts struct {
	Opened   int `json:"opened"`
	Resolved int `json:"resolved"`
}

// Standing is how long the open problems have lasted and whether the last week was better than nothing.
type Standing struct {
	// Tracked is the open problems (not open ports, not accepted) that have a history record; the rest of the numbers are about them.
	Tracked    int `json:"tracked"`
	OldestDays int `json:"oldest_days"`
	MedianDays int `json:"median_days"`
	OverMonth  int `json:"over_month"`
	// CameBack is how many of them have gone away and returned at least once.
	CameBack int `json:"came_back"`
	// Week is the last seven days and Previous the seven before; Trend compares problems appeared with problems gone this week.
	Week     WeekCounts `json:"week"`
	Previous WeekCounts `json:"previous"`
	Trend    string     `json:"trend"`
	// HistorySince is when the first record starts (the history begins when this version was installed); Enough is whether it covers
	// two weeks, so the comparison means something.
	HistorySince time.Time `json:"history_since,omitzero"`
	Enough       bool      `json:"enough"`
}

// StandingOf builds the standing. It reads nothing and keeps nothing.
func StandingOf(in StandingInput) Standing {
	s := Standing{Trend: TrendUnknown}
	byKey := make(map[string]models.FindingLife, len(in.Life))
	for _, l := range in.Life {
		byKey[l.Key] = l
		if s.HistorySince.IsZero() || l.FirstSeen.Before(s.HistorySince) {
			s.HistorySince = l.FirstSeen
		}
	}
	s.openProblems(in, byKey)
	s.week(in)
	return s
}

// openProblems measures the problems the last scan still shows and the operator has not accepted.
func (s *Standing) openProblems(in StandingInput, byKey map[string]models.FindingLife) {
	var ages []int
	for _, f := range ApplyAcks(in.Baseline.Findings, in.Acks, in.Now).Visible {
		l, ok := byKey[f.Key()]
		if f.Check == CheckOpenPort || !ok || !l.Open() {
			continue
		}
		days := int(in.Now.Sub(l.OpenSince).Hours() / 24)
		ages = append(ages, max(days, 0))
		if days > OverMonthDays {
			s.OverMonth++
		}
		if l.Returned > 0 {
			s.CameBack++
		}
	}
	s.Tracked = len(ages)
	if len(ages) == 0 {
		return
	}
	slices.Sort(ages)
	s.OldestDays, s.MedianDays = ages[len(ages)-1], ages[len(ages)/2]
}

// week counts what appeared and went away in the last seven days and the seven before, and sets the arrow.
func (s *Standing) week(in StandingInput) {
	s.Enough = !s.HistorySince.IsZero() && in.Now.Sub(s.HistorySince) >= standingEnough
	for _, l := range in.Life {
		s.Week = count(s.Week, l, in.Now, 0)
		s.Previous = count(s.Previous, l, in.Now, standingWeek)
	}
	if !s.Enough {
		return
	}
	switch net := s.Week.Opened - s.Week.Resolved; {
	case net < 0:
		s.Trend = TrendBetter
	case net > 0:
		s.Trend = TrendWorse
	default:
		s.Trend = TrendSame
	}
}

// count adds a record to the week that ended `back` ago: opened when its first appearance fell in it, resolved when it went away in it.
func count(w WeekCounts, l models.FindingLife, now time.Time, back time.Duration) WeekCounts {
	end := now.Add(-back)
	start := end.Add(-standingWeek)
	if l.FirstSeen.After(start) && !l.FirstSeen.After(end) {
		w.Opened++
	}
	if !l.Open() && l.ResolvedAt.After(start) && !l.ResolvedAt.After(end) {
		w.Resolved++
	}
	return w
}
