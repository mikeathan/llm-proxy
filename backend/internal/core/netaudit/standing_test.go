package netaudit

import (
	"testing"
	"time"

	"llm-proxy/models"
)

var standingNow = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func sAgo(days int) time.Time { return standingNow.AddDate(0, 0, -days) }

func standingBase(findings ...Finding) models.ScanBaseline {
	return models.ScanBaseline{ScannedAt: standingNow.Add(-time.Hour), Findings: findings}
}

func openLife(f Finding, firstDays, openDays, returned int) models.FindingLife {
	return models.FindingLife{Key: f.Key(), FirstSeen: sAgo(firstDays), OpenSince: sAgo(openDays), LastSeen: sAgo(0), Returned: returned}
}

func closedLife(f Finding, firstDays, resolvedDays int) models.FindingLife {
	return models.FindingLife{Key: f.Key(), FirstSeen: sAgo(firstDays), OpenSince: sAgo(firstDays), LastSeen: sAgo(resolvedDays), ResolvedAt: sAgo(resolvedDays), ResolvedBy: models.LifeByAudit}
}

func TestStanding_HowLongOpenProblemsHaveBeenThereAndHowManyCameBack(t *testing.T) {
	a, b, c := smbFinding("10.0.0.1"), smbFinding("10.0.0.2"), smbFinding("10.0.0.3")
	life := []models.FindingLife{openLife(a, 90, 90, 0), openLife(b, 40, 10, 2), openLife(c, 5, 5, 0)}
	s := StandingOf(StandingInput{Now: standingNow, Baseline: standingBase(a, b, c), Life: life})
	if s.Tracked != 3 || s.OldestDays != 90 || s.MedianDays != 10 || s.OverMonth != 1 || s.CameBack != 1 {
		t.Errorf("%+v", s)
	}
}

// An open port is a fact, not a problem, and an accepted finding is the operator's decision: neither counts as "broken".
func TestStanding_OpenPortsAndAcceptedFindingsAreNotCounted(t *testing.T) {
	problem := smbFinding("10.0.0.1")
	port := Finding{Device: "10.0.0.1", Port: 80, Proto: "tcp", Check: CheckOpenPort, Severity: SeverityInfo}
	accepted := smbFinding("10.0.0.9")
	ack := models.ScanAck{Key: accepted.Key(), AcceptedAt: standingNow.Add(-time.Hour), ExpiresAt: standingNow.Add(24 * time.Hour)}
	life := []models.FindingLife{openLife(problem, 30, 30, 0), openLife(port, 30, 30, 0), openLife(accepted, 30, 30, 0)}
	s := StandingOf(StandingInput{Now: standingNow, Baseline: standingBase(problem, port, accepted), Life: life, Acks: []models.ScanAck{ack}})
	if s.Tracked != 1 {
		t.Errorf("only the one open problem counts: %+v", s)
	}
}

func TestStanding_TheWeekIsCountedFromTheHistoryAndTheArrowFollowsIt(t *testing.T) {
	var life []models.FindingLife
	mk := func(i int) Finding { return smbFinding("10.0.1." + string(rune('0'+i))) }
	// History reaches back 30 days, so there is enough to compare two weeks.
	life = append(life, closedLife(mk(0), 30, 25))
	// This week: two problems opened, three went away.
	life = append(life, openLife(mk(1), 3, 3, 0), openLife(mk(2), 2, 2, 0))
	life = append(life, closedLife(mk(3), 20, 1), closedLife(mk(4), 20, 2), closedLife(mk(5), 20, 6))
	s := StandingOf(StandingInput{Now: standingNow, Baseline: standingBase(mk(1), mk(2)), Life: life})
	if !s.Enough || s.Week.Opened != 2 || s.Week.Resolved != 3 || s.Trend != TrendBetter {
		t.Errorf("%+v", s)
	}
	if s.Previous.Opened != 0 || s.Previous.Resolved != 0 {
		t.Errorf("previous week: %+v", s.Previous)
	}
}

func TestStanding_WorseAndSame(t *testing.T) {
	old := closedLife(smbFinding("10.0.9.9"), 30, 29)
	worse := StandingOf(StandingInput{Now: standingNow, Baseline: standingBase(smbFinding("10.0.2.1")), Life: []models.FindingLife{old, openLife(smbFinding("10.0.2.1"), 2, 2, 0)}})
	if worse.Trend != TrendWorse {
		t.Errorf("%+v", worse)
	}
	same := StandingOf(StandingInput{Now: standingNow, Baseline: standingBase(), Life: []models.FindingLife{old}})
	if same.Trend != TrendSame {
		t.Errorf("%+v", same)
	}
}

// The history only starts when this version was installed: with less than two weeks there is no arrow, and the start is stated.
func TestStanding_TooLittleHistoryHasNoArrowAndSaysWhereItStarts(t *testing.T) {
	f := smbFinding("10.0.3.1")
	s := StandingOf(StandingInput{Now: standingNow, Baseline: standingBase(f), Life: []models.FindingLife{openLife(f, 4, 4, 0)}})
	if s.Enough || s.Trend != TrendUnknown || !s.HistorySince.Equal(sAgo(4)) {
		t.Errorf("%+v", s)
	}
	if empty := StandingOf(StandingInput{Now: standingNow}); empty.Trend != TrendUnknown || !empty.HistorySince.IsZero() {
		t.Errorf("no history at all: %+v", empty)
	}
}
