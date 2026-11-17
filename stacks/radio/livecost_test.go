package radio

import (
	"testing"
	"time"
)

// Measures what one look at the radar costs when the air is busy: devices stored and heard now, over several cycles so each has a history. The
// numbers are logged (go test -run LiveCost -v) and the test fails only when a look takes longer than the page can wait.
func TestLiveCostStaysUnderASecondWhenTheAirIsBusy(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds thousands of devices")
	}
	if underRace {
		t.Skip("a wall-clock limit means nothing under the race detector, and seeding 3000 devices takes ~50s there")
	}
	for _, n := range []int{200, 1000, 3000} {
		r := devRig(t)
		rec, _, _ := r.s.recorder(r.s.h)
		frames := make([][]byte, n)
		for i := range frames {
			frames[i] = dataAt(MAC{0x02, 0x11, 0x22, byte(i >> 16), byte(i >> 8), byte(i)}, -40-i%50)
		}
		for c := 0; c < 3; c++ {
			cycleOf(t, rec, r.now, frames...)
		}
		rec.BeginCycle()
		for _, f := range frames {
			record(t, rec, f, r.now)
		}
		start := time.Now()
		v := liveOf(t, r)
		took := time.Since(start)
		size := len(mustJSON(t, v))
		t.Logf("%d devices heard now: %d dots, %d KB, one look %v", n, len(v.Devices), size/1024, took.Round(time.Millisecond))
		if took > time.Second {
			t.Errorf("%d devices: one look took %v, more than a second", n, took)
		}
	}
}
