package phonehome

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/models"
)

const (
	macA = "aa:bb:cc:00:00:01"
	macB = "aa:bb:cc:00:00:02"
)

var day0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

type memStore struct {
	state  models.PhoneHome
	writes int
	err    error
}

func (m *memStore) Read() (models.PhoneHome, error) { return m.state, m.err }
func (m *memStore) Write(s models.PhoneHome) error  { m.state = s; m.writes++; return m.err }

func q(name string) routerlog.Query {
	return routerlog.Query{Type: "A", Name: name, From: netip.MustParseAddr("192.168.50.188")}
}

func newWatch(t *testing.T) (*Watch, *memStore) {
	t.Helper()
	st := &memStore{}
	w, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	return w, st
}

func nameOf(s models.PhoneHome, mac, name string) *models.PhoneHomeName {
	for i := range s.Devices {
		if s.Devices[i].MAC == mac {
			for j := range s.Devices[i].Names {
				if s.Devices[i].Names[j].Name == name {
					return &s.Devices[i].Names[j]
				}
			}
		}
	}
	return nil
}

func TestObserve_RemembersWhichDeviceAskedForWhichNameAndWhen(t *testing.T) {
	w, _ := newWatch(t)
	w.Observe(macA, q("example.com"), day0)
	w.Observe(macA, q("example.com"), day0.Add(time.Hour))
	s := w.Snapshot()
	n := nameOf(s, macA, "example.com")
	if n == nil || n.Count != 2 || !n.First.Equal(day0) || !n.Last.Equal(day0.Add(time.Hour)) {
		t.Fatalf("name = %+v", n)
	}
	if s.Since.IsZero() || len(s.Devices) != 1 || !s.Devices[0].First.Equal(day0) {
		t.Fatalf("state = %+v", s)
	}
}

func TestObserve_CountsOneLookupOnceWhenTheRouterLogsAAndAAAA(t *testing.T) {
	w, _ := newWatch(t)
	w.Observe(macA, routerlog.Query{Type: "A", Name: "example.com"}, day0)
	w.Observe(macA, routerlog.Query{Type: "AAAA", Name: "example.com"}, day0)
	w.Observe(macA, routerlog.Query{Type: "AAAA", Name: "example.com"}, day0.Add(time.Second))
	if n := nameOf(w.Snapshot(), macA, "example.com"); n.Count != 1 {
		t.Fatalf("count = %d, want 1: the pair of lines is one lookup", n.Count)
	}
	w.Observe(macA, routerlog.Query{Type: "A", Name: "example.com"}, day0.Add(time.Minute))
	if n := nameOf(w.Snapshot(), macA, "example.com"); n.Count != 2 {
		t.Fatalf("count = %d, want 2: a later lookup counts", n.Count)
	}
}

func TestObserve_IgnoresALookupWithNoHardwareAddress(t *testing.T) {
	w, _ := newWatch(t)
	w.Observe("", q("example.com"), day0)
	w.Observe("not a mac", q("example.com"), day0)
	if s := w.Snapshot(); len(s.Devices) != 0 || !s.Since.IsZero() {
		t.Fatalf("state = %+v; a device that cannot be told apart by its hardware address is not kept", s)
	}
}

func TestObserve_KeepsEverythingBounded(t *testing.T) {
	w, _ := newWatch(t)
	for i := 0; i < maxNames+50; i++ {
		w.Observe(macA, q(label(i)+".example.com"), day0.Add(time.Duration(i)*time.Minute))
	}
	s := w.Snapshot()
	if got := len(s.Devices[0].Names); got != maxNames {
		t.Fatalf("names = %d, want %d", got, maxNames)
	}
	if nameOf(s, macA, label(0)+".example.com") != nil {
		t.Error("the name asked for longest ago must be the one dropped")
	}
	for i := 0; i < maxDevices+10; i++ {
		w.Observe(mac(i), q("example.com"), day0.Add(time.Duration(i)*time.Second))
	}
	if got := len(w.Snapshot().Devices); got != maxDevices {
		t.Fatalf("devices = %d, want %d", got, maxDevices)
	}
}

func label(i int) string {
	return "n" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26))
}

func mac(i int) string {
	return "02:00:00:00:" + string("0123456789abcdef"[(i>>4)&15]) + string("0123456789abcdef"[(i>>8)&15]) + ":" + string("0123456789abcdef"[(i>>4)&15]) + string("0123456789abcdef"[i&15])
}

func TestFlush_WritesOnlyWhenSomethingChangedAndSurvivesARestart(t *testing.T) {
	w, st := newWatch(t)
	if err := w.Flush(); err != nil || st.writes != 0 {
		t.Fatalf("an unchanged watch must not write: err = %v writes = %d", err, st.writes)
	}
	w.Observe(macA, q("example.com"), day0)
	if err := w.Flush(); err != nil || st.writes != 1 {
		t.Fatalf("flush: err = %v writes = %d", err, st.writes)
	}
	if err := w.Flush(); err != nil || st.writes != 1 {
		t.Fatalf("a second flush with nothing new wrote again: %d", st.writes)
	}
	again, err := New(st)
	if err != nil || nameOf(again.Snapshot(), macA, "example.com") == nil {
		t.Fatalf("after a restart: %v %+v", err, again.Snapshot())
	}
}

func TestFlush_AFailedWriteKeepsTheChangeForTheNextTry(t *testing.T) {
	w, st := newWatch(t)
	st.err = errors.New("disk full")
	w.Observe(macA, q("example.com"), day0)
	if err := w.Flush(); err == nil {
		t.Fatal("a failed write must be reported")
	}
	st.err = nil
	if err := w.Flush(); err != nil || st.writes != 2 {
		t.Fatalf("retry: err = %v writes = %d, want the change written now", err, st.writes)
	}
}

func TestNew_AnUnreadableRecordIsAnError(t *testing.T) {
	if _, err := New(&memStore{err: errors.New("corrupt")}); err == nil {
		t.Fatal("an unreadable record must not read as an empty one")
	}
}

// learned is a device that has been watched for the learning week and a day, so a new name is news.
func learned(w *Watch) time.Time {
	w.Observe(macA, q("ntp.example.net"), day0)
	return day0.Add(LearnFor + 24*time.Hour)
}

func TestEvaluate_ANewNameIsNothingDuringTheLearningWeekAndNewAfterIt(t *testing.T) {
	w, _ := newWatch(t)
	w.Observe(macA, q("ntp.example.net"), day0)
	w.Observe(macA, q("first-week.example.com"), day0.Add(2*24*time.Hour))
	during := day0.Add(3 * 24 * time.Hour)
	if got := Evaluate(w.Snapshot(), nil, during); len(got) != 0 {
		t.Fatalf("signals = %+v during the learning week", got)
	}
	after := day0.Add(LearnFor + 24*time.Hour)
	w.Observe(macA, q("ntp.example.net"), after)
	w.Observe(macA, q("c2.example.org"), after)
	got := Evaluate(w.Snapshot(), nil, after.Add(time.Hour))
	if len(got) != 1 || got[0].Kind != NewPlace || got[0].MAC != macA || !slices.Equal(got[0].Names, []string{"c2.example.org"}) {
		t.Fatalf("signals = %+v, want one new place naming only the new name", got)
	}
}

func TestEvaluate_ANewPlaceStaysNewForAWeekThenBecomesOrdinary(t *testing.T) {
	w, _ := newWatch(t)
	after := learned(w)
	w.Observe(macA, q("c2.example.org"), after)
	if got := Evaluate(w.Snapshot(), nil, after.Add(6*24*time.Hour)); len(got) != 1 {
		t.Fatalf("signals = %+v on day six", got)
	}
	if got := Evaluate(w.Snapshot(), nil, after.Add(8*24*time.Hour)); len(got) != 0 {
		t.Fatalf("signals = %+v on day eight: the name is no longer new", got)
	}
}

func beatingWatch(t *testing.T, name string, every time.Duration, n int) (*Watch, time.Time) {
	t.Helper()
	w, _ := newWatch(t)
	after := learned(w)
	at := after
	for i := 0; i < n; i++ {
		w.Observe(macA, q(name), at)
		at = at.Add(every)
	}
	return w, at
}

func TestEvaluate_ABeatingNewNameIsBeaconingAndAnIrregularOneIsNot(t *testing.T) {
	w, at := beatingWatch(t, "beacon.example.org", 5*time.Minute, 20)
	got := Evaluate(w.Snapshot(), nil, at)
	kinds := map[Kind]bool{}
	for _, s := range got {
		kinds[s.Kind] = true
	}
	if !kinds[Beaconing] || !kinds[NewPlace] {
		t.Fatalf("signals = %+v, want beaconing and new place", got)
	}

	w, _ = newWatch(t)
	after := learned(w)
	offsets := []time.Duration{0, 1 * time.Minute, 9 * time.Minute, 11 * time.Minute, 42 * time.Minute, 43 * time.Minute, 90 * time.Minute, 95 * time.Minute, 180 * time.Minute, 181 * time.Minute, 300 * time.Minute, 330 * time.Minute, 400 * time.Minute}
	for _, o := range offsets {
		w.Observe(macA, q("browse.example.org"), after.Add(o))
	}
	for _, s := range Evaluate(w.Snapshot(), nil, after.Add(8*time.Hour)) {
		if s.Kind == Beaconing {
			t.Fatalf("an irregular name was called beaconing: %+v", s)
		}
	}
}

func TestEvaluate_ALongKnownNameRefreshedOnABeatIsNeverBeaconing(t *testing.T) {
	w, _ := newWatch(t)
	for i := 0; i < 30; i++ { // NTP-style refresh from the first day
		w.Observe(macA, q("time.example.net"), day0.Add(time.Duration(i)*time.Hour))
	}
	now := day0.Add(LearnFor + 3*24*time.Hour)
	for i := 0; i < 30; i++ {
		w.Observe(macA, q("time.example.net"), now.Add(time.Duration(i)*time.Minute*5))
	}
	if got := Evaluate(w.Snapshot(), nil, now.Add(3*time.Hour)); len(got) != 0 {
		t.Fatalf("signals = %+v; a name known since the learning week is expected to refresh", got)
	}
}

func TestEvaluate_ADeviceThatIsOnlineAndNeverAsksDNSIsReportedOnceTheWatchHasWatchedADay(t *testing.T) {
	w, _ := newWatch(t)
	w.Observe(macA, q("example.com"), day0)
	online := InventoryDevice{MAC: macB, FirstSeen: day0.Add(-72 * time.Hour), LastSeen: day0.Add(30 * time.Hour)}
	now := day0.Add(30*time.Hour + 10*time.Minute)
	got := Evaluate(w.Snapshot(), []InventoryDevice{online}, now)
	if len(got) != 1 || got[0].Kind != AroundDNS || got[0].MAC != macB {
		t.Fatalf("signals = %+v, want the silent device", got)
	}
	// Before the watch has watched a day it says nothing.
	if got := Evaluate(w.Snapshot(), []InventoryDevice{online}, day0.Add(10*time.Hour)); len(got) != 0 {
		t.Fatalf("signals = %+v after ten hours", got)
	}
	// A device that is offline, or has asked lately, or is new itself, is not reported.
	offline := InventoryDevice{MAC: macB, FirstSeen: day0.Add(-72 * time.Hour), LastSeen: day0.Add(2 * time.Hour)}
	young := InventoryDevice{MAC: macB, FirstSeen: now.Add(-3 * time.Hour), LastSeen: now}
	asked := InventoryDevice{MAC: macA, FirstSeen: day0.Add(-72 * time.Hour), LastSeen: now}
	w.Observe(macA, q("example.com"), now.Add(-time.Hour))
	for name, d := range map[string]InventoryDevice{"offline": offline, "young": young, "asked": asked} {
		if got := Evaluate(w.Snapshot(), []InventoryDevice{d}, now); len(got) != 0 {
			t.Errorf("%s: signals = %+v", name, got)
		}
	}
}

func TestEvaluate_DoesNotChangeTheState(t *testing.T) {
	w, at := beatingWatch(t, "beacon.example.org", 5*time.Minute, 20)
	before := w.Snapshot()
	Evaluate(before, nil, at)
	if nameOf(before, macA, "beacon.example.org").Count != 20 {
		t.Fatal("the state was changed")
	}
}

func TestRun_WritesWhatWasObservedWhenItsContextEnds(t *testing.T) {
	w, st := newWatch(t)
	w.Observe(macA, q("example.com"), day0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx, nil); close(done) }()
	cancel()
	<-done
	if st.writes != 1 {
		t.Fatalf("writes = %d, want the last changes written at shutdown", st.writes)
	}
}

func TestNewEmpty_StartsWithNothingAndWritesOnTheFirstChange(t *testing.T) {
	st := &memStore{err: errors.New("corrupt")}
	w := NewEmpty(st)
	if len(w.Snapshot().Devices) != 0 {
		t.Fatal("not empty")
	}
	st.err = nil
	w.Observe(macA, q("example.com"), day0)
	if err := w.Flush(); err != nil || st.writes != 1 {
		t.Fatalf("flush: %v, writes %d", err, st.writes)
	}
}

func TestMarkTold_RemembersAKeyOnceAndKeepsTheRecordBounded(t *testing.T) {
	w, st := newWatch(t)
	w.MarkTold("k0", day0)
	w.MarkTold("k0", day0)
	if !w.WasTold("k0") || w.WasTold("other") || len(w.Snapshot().Told) != 1 {
		t.Fatalf("told = %+v", w.Snapshot().Told)
	}
	for i := 0; i < maxTold+10; i++ {
		w.MarkTold(label(i), day0.Add(time.Duration(i)*time.Minute))
	}
	if got := len(w.Snapshot().Told); got != maxTold {
		t.Fatalf("told = %d, want %d", got, maxTold)
	}
	if w.WasTold("k0") {
		t.Error("the oldest entry must be the one dropped")
	}
	if err := w.Flush(); err != nil || len(st.state.Told) != maxTold {
		t.Fatalf("flush: %v, stored %d", err, len(st.state.Told))
	}
}

func TestKnownBad_ReportsRecentNamesOnTheListWhateverTheDevicesAge(t *testing.T) {
	w, _ := newWatch(t)
	w.Observe(macA, q("a.g6.wtf"), day0)         // asked in the learning week: still bad
	w.Observe(macA, q("sub.bad.example"), day0)  // under a listed name
	w.Observe(macA, q("fine.example.com"), day0) // not listed
	w.Observe(macB, q("a.g6.wtf"), day0.Add(-30*24*time.Hour))
	match := func(name string) (string, bool) {
		switch name {
		case "a.g6.wtf":
			return name, true
		case "sub.bad.example":
			return "bad.example", true
		}
		return "", false
	}
	got := KnownBad(w.Snapshot(), match, day0.Add(time.Hour))
	if len(got) != 1 || got[0].Kind != KnownBadName || got[0].MAC != macA || len(got[0].Names) != 2 || got[0].Names[0] != "a.g6.wtf" || got[0].Names[1] != "sub.bad.example" {
		t.Fatalf("signals = %+v, want one for the first device naming the two listed names it asked for", got)
	}
	if got := KnownBad(w.Snapshot(), match, day0.Add(9*24*time.Hour)); len(got) != 0 {
		t.Fatalf("signals = %+v nine days later: an old lookup is no longer news", got)
	}
	if got := KnownBad(w.Snapshot(), nil, day0); len(got) != 0 {
		t.Fatal("no list, no signal")
	}
}

func TestWithKnownBad_TakesAListedNameOutOfTheNewPlaceSignalOfTheSameDevice(t *testing.T) {
	signals := []Signal{{Kind: NewPlace, MAC: macA, Names: []string{"bad.example", "fresh.example"}}, {Kind: Beaconing, MAC: macA, Names: []string{"bad.example"}}, {Kind: NewPlace, MAC: macB, Names: []string{"bad.example"}}}
	bad := []Signal{{Kind: KnownBadName, MAC: macA, Names: []string{"bad.example"}}}
	got := WithKnownBad(signals, bad)
	if len(got) != 3 || got[0].Names[0] != "fresh.example" || len(got[0].Names) != 1 || got[1].MAC != macB || got[2].Kind != KnownBadName {
		t.Fatalf("signals = %+v", got)
	}
}

// slowStore blocks a write until released, to show what a slow disk does to the listener.
type slowStore struct {
	memStore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *slowStore) Write(m models.PhoneHome) error {
	s.once.Do(func() { close(s.entered); <-s.release })
	return s.memStore.Write(m)
}

func TestFlush_ASlowWriteNeverHoldsUpObserve(t *testing.T) {
	st := &slowStore{entered: make(chan struct{}), release: make(chan struct{})}
	w, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	w.Observe(macA, q("example.com"), day0)
	done := make(chan error)
	go func() { done <- w.Flush() }()
	<-st.entered // the write is in progress and blocked
	observed := make(chan struct{})
	go func() { w.Observe(macA, q("other.example.com"), day0.Add(time.Minute)); close(observed) }()
	select {
	case <-observed:
	case <-time.After(2 * time.Second):
		t.Fatal("Observe waited for the disk write: the router listener would stop reading while the names are saved")
	}
	close(st.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if nameOf(st.state, macA, "other.example.com") == nil {
		t.Fatal("a lookup made during the write must be kept by the next flush")
	}
}

func TestFlush_AFailedWriteIsRetriedWhileMoreArrives(t *testing.T) {
	w, st := newWatch(t)
	st.err = errors.New("disk full")
	w.Observe(macA, q("example.com"), day0)
	if err := w.Flush(); err == nil {
		t.Fatal("must report")
	}
	st.err = nil
	w.Observe(macA, q("more.example.com"), day0.Add(time.Minute))
	if err := w.Flush(); err != nil || nameOf(st.state, macA, "example.com") == nil || nameOf(st.state, macA, "more.example.com") == nil {
		t.Fatalf("flush = %v, stored %+v", err, st.state)
	}
}

func TestBounds_KeepTheWorstCaseSmall(t *testing.T) {
	// A bound that is too loose is a bound that does not protect: at 256 devices of 512 names the watch held 258 MB and wrote a 114 MB file.
	if worst := maxDevices * maxNames * maxTimes; worst > 128*256*16 {
		t.Fatalf("the bounds allow %d kept times, more than the reviewed worst case", worst)
	}
}
