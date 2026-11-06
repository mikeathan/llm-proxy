package netaudit

import (
	"errors"
	"testing"
	"time"
)

func TestRunHealth_AStepThatNeverRanIsNeitherGoodNorFailed(t *testing.T) {
	r := NewRunHealth().Reading("x", true, time.Hour)
	if !r.On || r.Failed || !r.LastGood.IsZero() || r.Limit != time.Hour {
		t.Errorf("reading = %+v", r)
	}
}

func TestRunHealth_AFailureKeepsTheLastGoodTimeAndAGoodRunClearsIt(t *testing.T) {
	h := NewRunHealth()
	clock := time.Date(2026, 11, 6, 2, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return clock }
	h.Record("x", nil)
	good := clock
	clock = clock.Add(24 * time.Hour)
	h.Record("x", errors.New("boom"))
	if r := h.Reading("x", true, time.Hour); !r.Failed || !r.LastGood.Equal(good) {
		t.Errorf("after a failure: %+v, want failed with the earlier good time %v", r, good)
	}
	h.Record("x", nil)
	if r := h.Reading("x", true, time.Hour); r.Failed || !r.LastGood.Equal(clock) {
		t.Errorf("after a good run: %+v", r)
	}
}

func TestRunHealth_StepsAreKeptApart(t *testing.T) {
	h := NewRunHealth()
	h.Record("a", errors.New("boom"))
	if h.Reading("b", true, time.Hour).Failed {
		t.Error("a failure of one step shows on another")
	}
}

func TestRunHealth_ANilRecordIsSafe(t *testing.T) {
	var h *RunHealth
	h.Record("x", errors.New("boom"))
	if r := h.Reading("x", true, time.Hour); r.Failed || !r.On {
		t.Errorf("reading = %+v", r)
	}
}
