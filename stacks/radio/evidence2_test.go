package radio

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestAQuietHoursSightingIsOneBitADayNeverTheHour(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	night := time.Date(2026, 11, 13, 3, 0, 0, 0, time.Local)
	r.BeginCycle()
	record(t, r, data(neighbour), night)
	if err := r.EndCycle(context.Background(), night); err != nil {
		t.Fatal(err)
	}
	day := night.Add(10 * time.Hour)
	r.BeginCycle()
	record(t, r, data(neighbour), day)
	if err := r.EndCycle(context.Background(), day); err != nil {
		t.Fatal(err)
	}
	if row := rowOf(t, s, neighbour); row.NightDays != 1 || row.Days != 1 {
		t.Errorf("one day, heard at night: %+v", row)
	}
	var cols int
	_ = s.db.QueryRow(`SELECT count(*) FROM pragma_table_info('daily') WHERE name LIKE '%hour%'`).Scan(&cols)
	if cols != 0 {
		t.Error("no hour is kept for anyone's daily row")
	}
}

func TestABigJumpBetweenCyclesCountsAsMovementAndAWobbleDoesNot(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	for i := range 6 {
		cycleOf(t, r, t0, dataAt(neighbour, -50-4*(i%2)), dataAt(mineMAC, -40-25*(i%2)))
	}
	still, moving := rowOf(t, s, neighbour), rowOf(t, s, mineMAC)
	if still.Crossings != 0 || moving.Crossings != 5 {
		t.Errorf("still %d, moving %d crossings", still.Crossings, moving.Crossings)
	}
	if f := FeaturesOf(moving, map[int64]int{dayNumber(t0): 6}); f.MoveShare < 0.8 {
		t.Errorf("moved in most cycles: %+v", f)
	}
}

func TestRecentDaysWeighMoreInHowOftenADeviceIsHeard(t *testing.T) {
	today := dayNumber(t0) + 6
	listened := map[int64]int{}
	for d := dayNumber(t0); d <= today; d++ {
		listened[d] = 100
	}
	// Heard in every cycle for the first five days, then gone for the last two: the plain share is 5/7, the weighted one much lower.
	w := 0.0
	for d := dayNumber(t0); d < dayNumber(t0)+5; d++ {
		w += 100 * recencyWeight(today-d)
	}
	r := FeatureRow{FirstDay: dayNumber(t0), Days: 5, Cycles: 500, WCycles: w, Today: today}
	f := FeaturesOf(r, listened)
	if plain := 500.0 / 700; f.Share >= plain-0.1 {
		t.Errorf("weighted share %.2f should be well under the plain %.2f", f.Share, plain)
	}
	if math.Abs(recencyWeight(2)-0.5) > 1e-9 || recencyWeight(-3) != 1 {
		t.Error("half the weight every two days, and a future day weighs one")
	}
}

func TestSignalsAreReadAgainstYourOwnAccessPoints(t *testing.T) {
	rows := []FeatureRow{{OwnAP: true, HasMax: true, SigMax: -60}, {OwnAP: true, HasMax: true, SigMax: -64}, {HasMax: true, SigMax: -90}}
	if got := signalOffset(rows); got != 15 {
		t.Errorf("a quiet adapter is shifted up, capped at 15 dB: %v", got)
	}
	if got := signalOffset(rows[2:]); got != 0 {
		t.Errorf("with no access point of yours nothing is shifted: %v", got)
	}
	v := vectorOf(DefaultRules(), Features{HasSignal: true, SignalMean: -70, SignalMax: -60}, DeviceUnknown, 10)
	if v.c[fSigMean] != -60 || v.c[fSigMax] != -50 {
		t.Errorf("%+v", v.c)
	}
}

func TestAFirstLookGuessesBeforeTheSortingCanAndStopsAfter(t *testing.T) {
	weak := DefaultRules().WeakDBm
	if got := firstLook(FeatureRow{HasMax: true, SigMax: weak - 5}, weak); got != FirstLookNeighbour {
		t.Errorf("never strong: %q", got)
	}
	if got := firstLook(FeatureRow{Random: true, HasMax: true, SigMax: -50}, weak); got != FirstLookMobile {
		t.Errorf("a private address: %q", got)
	}
	if got := firstLook(FeatureRow{Random: true, WifiGen: 6}, weak); got != "" {
		t.Errorf("an access point on a private address is not a phone: %q", got)
	}
	if got := firstLook(FeatureRow{Class: "personal", Random: true}, weak); got != "" {
		t.Errorf("a sorted device has no first look: %q", got)
	}
}

func TestAnAccessPointYouStopConfirmingLosesItsMark(t *testing.T) {
	s, _ := openTest(t, 0)
	r := NewRecorder(fakeVault{}, s)
	confirmHomeAP(t, s, r, false)
	cycle(t, r, data(macAP), beacon("home"))
	if !rowOf(t, s, macAP).OwnAP {
		t.Fatal("your confirmed access point is marked")
	}
	apTag, _ := fakeVault{}.Tag(context.Background(), tagAP, macAP[:])
	if _, err := s.SetOwnAPConfirmed(context.Background(), apTag, false); err != nil {
		t.Fatal(err)
	}
	if err := r.RefreshOwnAPs(context.Background()); err != nil {
		t.Fatal(err)
	}
	cycle(t, r, data(macAP), beacon("home"))
	if rowOf(t, s, macAP).OwnAP {
		t.Error("withdrawn, the mark goes at the next cycle it is heard")
	}
}
