package radio

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"llm-proxy-stacks/sdk"
)

// scriptedPriv answers each radio_listen with the next scripted result and notes what was asked.
type scriptedPriv struct {
	mu     sync.Mutex
	asked  []ListenParams
	grants []string
	next   func(n int, p ListenParams) (stacksdk.PrivilegedResult, error)
}

func (p *scriptedPriv) Run(_ context.Context, req stacksdk.PrivilegedRequest) (stacksdk.PrivilegedResult, error) {
	var lp ListenParams
	_ = json.Unmarshal(req.Params, &lp)
	p.mu.Lock()
	p.asked = append(p.asked, lp)
	p.grants = append(p.grants, req.Grant)
	n := len(p.asked)
	p.mu.Unlock()
	if req.Action != "radio_listen" {
		panic("the scheduler asked for " + req.Action)
	}
	return p.next(n, lp)
}

func (p *scriptedPriv) count() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.asked) }

func okWindow(int, ListenParams) (stacksdk.PrivilegedResult, error) {
	return stacksdk.PrivilegedResult{Kind: "bytes", Data: windowBytes(0, [][]byte{data(neighbour), beacon("home")}, true)}, nil
}

type listenRig struct {
	s    *Stack
	priv *scriptedPriv
	dir  *pathDir
	now  time.Time
	mu   sync.Mutex
	nap  chan time.Duration // each wait between cycles, as asked
	step chan struct{}      // the test lets a wait finish
}

func newRig(t *testing.T, doc string, next func(int, ListenParams) (stacksdk.PrivilegedResult, error)) *listenRig {
	t.Helper()
	r := &listenRig{priv: &scriptedPriv{next: next}, dir: &pathDir{dir: t.TempDir()}, now: t0, nap: make(chan time.Duration, 64), step: make(chan struct{}, 64)}
	r.s = New(fstest.MapFS{})
	r.s.clock = func() time.Time { r.mu.Lock(); defer r.mu.Unlock(); return r.now }
	r.s.sleep = func(ctx context.Context, d time.Duration) error {
		r.nap <- d
		select {
		case <-r.step:
			r.mu.Lock()
			r.now = r.now.Add(d)
			r.mu.Unlock()
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	h := host(doc, r.dir)
	h.Privileged, h.Vault = r.priv, fakeVault{}
	if err := r.s.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.s.Stop(context.Background()) })
	return r
}

func (r *listenRig) post(t *testing.T, body string) stacksdk.Response {
	t.Helper()
	resp, err := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPost, Path: ListenPath, Body: []byte(body)})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (r *listenRig) state(t *testing.T) ListenState {
	t.Helper()
	resp, err := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: ListenPath})
	if err != nil {
		t.Fatal(err)
	}
	var st ListenState
	if err := json.Unmarshal(resp.Body, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

const twoChannels = `{"enabled":true,"channels":[1,60],"window_seconds":20,"cycle_seconds":300}`

func TestPlanDefaultsAndCaps(t *testing.T) {
	p := Settings{}.Plan()
	if len(p.Channels) == 0 || p.WindowSeconds != 20 || p.CycleSeconds != 300 {
		t.Errorf("defaults: %+v", p)
	}
	big := Settings{Channels: []int{1, 1, 6, 60, 60, 999, -3}, WindowSeconds: 500, CycleSeconds: 1}.Plan()
	if big.WindowSeconds != 30 || big.CycleSeconds != 60 || len(big.Channels) != 3 {
		t.Errorf("a duplicate or an impossible channel is dropped and the numbers are held to their caps: %+v", big)
	}
	many := make([]int, 0, 30)
	for c := 1; c <= 13; c++ {
		many = append(many, c)
	}
	many = append(many, 36, 40, 44, 48, 52, 56, 60, 64, 149, 153, 157, 161)
	capped := Settings{Channels: many, CycleSeconds: 60}.Plan()
	if got := len(capped.Channels) * 3600 / capped.CycleSeconds; got > MaxWindowsPerHour {
		t.Errorf("%d windows an hour is over what the helper allows (%d)", got, MaxWindowsPerHour)
	}
}

func TestRequestListIsTheOneListTheSessionApprovesAndTheSchedulerAsks(t *testing.T) {
	plan := Settings{Channels: []int{1, 60}, WindowSeconds: 20}.Plan()
	list := plan.Requests("wlx00")
	if len(list) != 2 || list[1].Action != "radio_listen" || string(list[1].Params) != `{"iface":"wlx00","channel":60,"seconds":20}` {
		t.Errorf("%+v", list)
	}
}

func TestListenWithNoSessionStopsAtOnceAndNeverRetries(t *testing.T) {
	r := newRig(t, twoChannels, func(int, ListenParams) (stacksdk.PrivilegedResult, error) {
		return stacksdk.PrivilegedResult{}, stacksdk.Errorf(stacksdk.CodeDenied, "no open session approved this")
	})
	if resp := r.post(t, `{"adapter":"wlx00","hours":1}`); resp.Status != http.StatusAccepted {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	waitFor(t, "the loop to stop", func() bool { return !r.state(t).Listening })
	if n := r.priv.count(); n != 1 {
		t.Errorf("a refusal must end the run, not be retried: asked %d times", n)
	}
	if st := r.state(t); st.Ended != EndedNoSession {
		t.Errorf("the page says why: %+v", st)
	}
	if r.priv.grants[0] != "" {
		t.Error("the scheduler holds no grant: it asks as a session does")
	}
}

func TestListenWalksTheChannelsEveryCycleAndKeepsWhatItHeard(t *testing.T) {
	r := newRig(t, twoChannels, okWindow)
	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "the first cycle", func() bool { return r.priv.count() == 2 })
	if d := <-r.nap; d != 300*time.Second-2*20*time.Second {
		t.Errorf("the cycle is 5 minutes from start to start, so the wait is what is left after two 20 s windows: %v", d)
	}
	st := r.state(t)
	if !st.Listening || len(st.Windows) != 2 || st.Windows[0].Channel != 1 || st.Windows[1].Channel != 60 {
		t.Fatalf("%+v", st)
	}
	if st.Windows[1].Recorded != 2 || !st.Windows[1].ChannelMatched || st.Windows[0].ChannelMatched {
		t.Errorf("channel 60 frames are on 60 and not on 1: %+v", st.Windows)
	}
	r.step <- struct{}{}
	waitFor(t, "the second cycle to finish", func() bool { return r.priv.count() == 4 && len(r.nap) == 1 })
	if st := r.state(t); st.Cycles < 2 || st.Until.IsZero() {
		t.Errorf("%+v", st)
	}
}

func TestListenStopsAtTheTimeCapAndAtTheFrameCap(t *testing.T) {
	r := newRig(t, `{"enabled":true,"channels":[60],"cycle_seconds":300}`, okWindow)
	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "a wait", func() bool { return len(r.nap) == 1 })
	r.mu.Lock()
	r.now = r.now.Add(2 * time.Hour)
	r.mu.Unlock()
	r.step <- struct{}{}
	waitFor(t, "the time cap", func() bool { return !r.state(t).Listening })
	if st := r.state(t); st.Ended != EndedTime {
		t.Errorf("%+v", st)
	}

	r2 := newRig(t, `{"enabled":true,"channels":[60]}`, okWindow)
	r2.s.maxFrames = 3
	r2.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "a wait", func() bool { return len(r2.nap) == 1 })
	r2.step <- struct{}{}
	waitFor(t, "a second wait", func() bool { return len(r2.nap) == 2 })
	r2.step <- struct{}{}
	waitFor(t, "the frame cap", func() bool { return !r2.state(t).Listening })
	if st := r2.state(t); st.Ended != EndedFrames {
		t.Errorf("%+v", st)
	}
}

func TestListenStopsAfterRepeatedFailuresAndSaysWhat(t *testing.T) {
	r := newRig(t, `{"enabled":true,"channels":[1,6,11,60]}`, func(int, ListenParams) (stacksdk.PrivilegedResult, error) {
		return stacksdk.PrivilegedResult{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the helper is not installed")
	})
	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "stop", func() bool { return !r.state(t).Listening })
	st := r.state(t)
	if st.Ended != EndedFailing || st.LastError != "the helper is not installed" || r.priv.count() != maxFailures {
		t.Errorf("%+v asked %d", st, r.priv.count())
	}
}

func TestListenOneBadWindowDoesNotEndTheRun(t *testing.T) {
	r := newRig(t, `{"enabled":true,"channels":[1,60]}`, func(n int, p ListenParams) (stacksdk.PrivilegedResult, error) {
		if n == 1 {
			return stacksdk.PrivilegedResult{Data: []byte("RWT1 not a window but long enough to carry a trailer-looking tail........")}, nil
		}
		return okWindow(n, p)
	})
	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "both windows", func() bool { return len(r.state(t).Windows) == 2 })
	st := r.state(t)
	if !st.Listening || st.Windows[0].Stop != "unreadable" || st.Windows[1].Recorded != 2 {
		t.Errorf("%+v", st)
	}
}

func TestListenIsRefusedWhenOffWhenRunningAndWithABadBody(t *testing.T) {
	r := newRig(t, `{"enabled":false}`, okWindow)
	if resp := r.post(t, `{"adapter":"wlx00","hours":1}`); resp.Status != http.StatusConflict {
		t.Errorf("off: %d %s", resp.Status, resp.Body)
	}
	on := newRig(t, twoChannels, okWindow)
	for _, bad := range []string{`{`, `{"adapter":"","hours":1}`, `{"adapter":"a b","hours":1}`, `{"adapter":"wlx00","hours":0}`, `{"adapter":"wlx00","hours":13}`, `{"adapter":"wlx00","hours":1,"x":1}`} {
		if resp := on.post(t, bad); resp.Status != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, resp.Status)
		}
	}
	if resp := on.post(t, `{"adapter":"wlx00","hours":1}`); resp.Status != http.StatusAccepted {
		t.Fatal(resp.Status)
	}
	waitFor(t, "a wait", func() bool { return len(on.nap) == 1 })
	if resp := on.post(t, `{"adapter":"wlx00","hours":1}`); resp.Status != http.StatusConflict {
		t.Errorf("one run at a time: %d", resp.Status)
	}
}

func TestListenStopRouteAndStackStopEndTheRunAndLeaveNoGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()
	r := newRig(t, twoChannels, okWindow)
	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "a wait", func() bool { return len(r.nap) == 1 })
	resp, _ := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodDelete, Path: ListenPath})
	if resp.Status != http.StatusOK {
		t.Fatal(resp.Status)
	}
	if st := r.state(t); st.Listening || st.Ended != EndedStopped {
		t.Errorf("%+v", st)
	}
	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "a wait", func() bool { return len(r.nap) >= 1 })
	if err := r.s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "goroutines to end", func() bool { return runtime.NumGoroutine() <= before })
}

func TestSwitchingTheStackOffEndsTheRun(t *testing.T) {
	r := newRig(t, twoChannels, okWindow)
	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "a wait", func() bool { return len(r.nap) == 1 })
	r.s.mu.Lock()
	r.s.h.Config = fakeConfig{`{"enabled":false}`}
	r.s.mu.Unlock()
	if err := r.s.ConfigChanged(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := r.state(t); st.Listening || st.Ended != EndedSwitchedOff {
		t.Errorf("%+v", st)
	}
}

func TestOnlyTheWindowsSummaryIsKept(t *testing.T) {
	r := newRig(t, `{"enabled":true,"channels":[60]}`, okWindow)
	r.post(t, `{"adapter":"wlx00","hours":1}`)
	waitFor(t, "a window", func() bool { return len(r.state(t).Windows) == 1 })
	resp, _ := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: ListenPath})
	for _, m := range []MAC{neighbour, macAP, macPhone} {
		if containsHex(resp.Body, m) {
			t.Errorf("the state of a run carries an address: %s", resp.Body)
		}
	}
	if _, err := filepath.Glob(filepath.Join(r.dir.dir, DBFile)); err != nil {
		t.Fatal(err)
	}
}

func containsHex(b []byte, m MAC) bool {
	for _, form := range []string{m.String(), string(m[:])} {
		if len(form) > 0 && bytes.Contains(b, []byte(form)) {
			return true
		}
	}
	return false
}
