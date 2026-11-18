package radio

import (
	"context"
	"math"
	"slices"
	"time"

	"llm-proxy-stacks/sdk"
)

// Focus windows: the run's idle time between two sweeps spent listening again on the channels the owner's devices use, so their dots move more
// often than once a cycle. The sweep is untouched: every channel is still heard every cycle, so a device nobody knows yet is found exactly as
// before, and every stored number (the day rows, the shares, the classes, the track) still comes from sweeps only. A focus window only moves the
// live picture: it writes nothing, and its readings are a short, bounded ring per device in memory, forgotten when the run ends.
const (
	// focusReadings is how many focus readings a device keeps (one a window it was heard in); focusKeepFor how long one is kept.
	focusReadings = 8
	focusKeepFor  = 15 * time.Minute
	// focusFramesPerHour is the focus windows' own frame budget, apart from the sweep's (framesPerHour). A focus frame costs a parse, a tag and
	// a sum: no database write (BenchmarkFocusIngest).
	focusFramesPerHour = 170_000
	// focusOverheadSeconds is the time a window takes beyond its listening (the helper puts the adapter in monitor mode and back): measured on
	// vertex at about 4 s, 2026-11-18.
	focusOverheadSeconds = 5
	// movingWithin is how far back the readings that say a device is moving reach.
	movingWithin = 3 * time.Minute
)

// focusReading is one focus window's mean signal of one device.
type focusReading struct {
	at      int64 // unix seconds the window ended
	signal  float64
	channel int
}

// FocusIngest reads one focus window into the live picture only: the senders heard on the channel asked for, each one mean signal, and nothing
// written. Beacons and probe answers are left to the sweep (an access point does not move), and so are frames with no signal.
func FocusIngest(ctx context.Context, rec *Recorder, raw []byte, asked WindowAsked, start time.Time) (WindowSummary, error) {
	w, err := DecodeWindow(raw)
	if err != nil {
		return WindowSummary{}, err
	}
	sum := WindowSummary{Channel: asked.Channel, Frames: len(w.Records), Stop: w.Trailer.Stop.String(), KernelSeen: w.Trailer.KernelSeen,
		KernelDropped: w.Trailer.KernelDropped, ElapsedMs: w.Trailer.ElapsedMs, At: start}
	heard := map[string]*agg{}
	for _, r := range w.Records {
		f, perr := Parse(r.Frame)
		if perr != nil {
			sum.Unread++
			continue
		}
		if channelOf(int(f.FreqMHz)) != asked.Channel {
			sum.OffChannel++
			continue
		}
		sum.OnChannel++
		if !focusable(f) {
			continue
		}
		tag, err := rec.tag(ctx, tagDevice, f.Transmitter[:])
		if err != nil {
			return sum, err
		}
		a := heard[string(tag)]
		if a == nil {
			if len(heard) >= liveMaxDevices {
				continue
			}
			a = &agg{}
			heard[string(tag)] = a
		}
		a.add(f)
		sum.Recorded++
	}
	sum.ChannelMatched = sum.OnChannel > 0 && sum.OffChannel == 0
	rec.addFocus(heard, start.Add(time.Duration(w.Trailer.ElapsedMs)*time.Millisecond), asked.Channel)
	return sum, nil
}

// focusable is a frame whose sender a focus window follows: a device's own frame (not a group address) with a signal.
func focusable(f Frame) bool {
	return f.Kind != KindBeacon && f.Kind != KindProbeResponse && f.HasTransmitter && f.Transmitter[0]&1 == 0 && f.SignalOK
}

// addFocus keeps one reading a device for a focus window, bounded in devices, readings and age.
func (r *Recorder) addFocus(heard map[string]*agg, at time.Time, channel int) {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	if r.focus == nil {
		r.focus = map[string][]focusReading{}
	}
	r.pruneFocus(at)
	for tag, a := range heard {
		if a.n == 0 {
			continue
		}
		pts, ok := r.focus[tag]
		if !ok && len(r.focus) >= liveMaxDevices {
			continue
		}
		pts = append(pts, focusReading{at: at.Unix(), signal: float64(a.sum) / float64(a.n), channel: channel})
		if len(pts) > focusReadings {
			pts = pts[len(pts)-focusReadings:]
		}
		r.focus[tag] = pts
	}
}

// pruneFocus forgets readings older than focusKeepFor. cmu is held.
func (r *Recorder) pruneFocus(now time.Time) {
	oldest := now.Add(-focusKeepFor).Unix()
	for tag, pts := range r.focus {
		i := 0
		for i < len(pts) && pts[i].at < oldest {
			i++
		}
		if i == len(pts) {
			delete(r.focus, tag)
		} else if i > 0 {
			r.focus[tag] = slices.Clone(pts[i:])
		}
	}
}

// focusSince is a device's focus readings after a time, oldest first. cmu is held; the slice is a copy.
func (r *Recorder) focusSince(tag string, after int64) []focusReading {
	var out []focusReading
	for _, p := range r.focus[tag] {
		if p.at > after {
			out = append(out, p)
		}
	}
	return out
}

// movingOf says whether a device's signal moved by crossJumpDB or more within movingWithin of its newest reading: the cycle means it has and
// its focus readings, newest last. It is for the picture only; the stored movement share still comes from sweeps (absorbCycle).
func movingOf(lastCycleSignal float64, hasCycle bool, recent []focusReading) bool {
	if len(recent) == 0 {
		return false
	}
	newest := recent[len(recent)-1].at
	lo, hi := math.Inf(1), math.Inf(-1)
	n := 0
	for _, p := range recent {
		if newest-p.at <= int64(movingWithin/time.Second) {
			lo, hi, n = math.Min(lo, p.signal), math.Max(hi, p.signal), n+1
		}
	}
	if hasCycle {
		lo, hi, n = math.Min(lo, lastCycleSignal), math.Max(hi, lastCycleSignal), n+1
	}
	return n >= 2 && hi-lo >= crossJumpDB
}

// focusPlan picks the focus windows' channels: each channel the owner's devices were heard on in the last sweep, weighted by how many of their
// frames it carried, in a fair order (the channel owed the most goes next). A channel with none of the owner's frames gets no focus window: the
// sweep already visits it every cycle.
type focusPlan struct {
	weight map[int]float64
	credit map[int]float64
	frames map[int]int // the last sweep's frames on each channel: what a focus window there is expected to read
}

func newFocusPlan(sweep []WindowSummary) *focusPlan {
	p := &focusPlan{weight: map[int]float64{}, credit: map[int]float64{}, frames: map[int]int{}}
	total := 0
	for _, w := range sweep {
		p.frames[w.Channel] = w.Frames
		if w.OwnFrames > 0 && w.ChannelMatched {
			p.weight[w.Channel] += float64(w.OwnFrames)
			total += w.OwnFrames
		}
	}
	for ch, wt := range p.weight {
		p.weight[ch] = wt / float64(total)
	}
	return p
}

// next is the channel to focus on, false when no channel earns one.
func (p *focusPlan) next() (int, bool) {
	if len(p.weight) == 0 {
		return 0, false
	}
	chans := make([]int, 0, len(p.weight))
	for ch := range p.weight {
		chans = append(chans, ch)
	}
	slices.Sort(chans) // a fixed order, so a tie goes the same way every time
	best := chans[0]
	for _, ch := range chans {
		p.credit[ch] += p.weight[ch]
		if p.credit[ch] > p.credit[best] {
			best = ch
		}
	}
	p.credit[best]--
	return best, true
}

// focusBudget is the focus windows' pace: frames may be read as fast as focusFramesPerHour allows over the run so far, plus one window's worth,
// so a run's first focus window is never refused by an empty clock.
type focusBudget struct {
	started time.Time
	frames  int
	windows int
}

// allows says whether a focus window expected to read this many frames fits the budget now.
func (b *focusBudget) allows(now time.Time, expected int) bool {
	hours := now.Sub(b.started).Hours()
	return float64(b.frames+expected) <= hours*focusFramesPerHour+float64(RadioMaxWindowFrames)
}

// RadioMaxWindowFrames is the most frames one window can return (the helper's cap, wire.RadioMaxFrames in the core).
const RadioMaxWindowFrames = 4000

// focusPerHour is how many focus windows an hour a plan may run: the time a cycle leaves after its sweep, and never past the windows an hour
// the helper allows beyond the sweep's.
func (p Plan) focusPerHour() int {
	sweep := len(p.Channels) * (p.WindowSeconds + focusOverheadSeconds)
	idle := p.CycleSeconds - sweep
	perCycle := max(idle, 0) / (p.WindowSeconds + focusOverheadSeconds)
	perHour := perCycle * 3600 / p.CycleSeconds
	return max(0, min(perHour, MaxWindowsPerHour-p.windowsPerHour()))
}

// sessionWindowsPerHour is every window an hour the run may ask for: the sweep's (windowsPerHour) and the focus windows'. A session is sized
// from it.
func (p Plan) sessionWindowsPerHour() int { return p.windowsPerHour() + p.focusPerHour() }

// focuser runs the focus windows of one run.
type focuser struct {
	s       *Stack
	priv    stacksdk.Privileged
	rec     *Recorder
	r       *run
	adapter string
	plan    Plan
	budget  focusBudget
	off     bool // a focus window was refused or failed: the rest of the run only sweeps
}

// until spends the time up to deadline on focus windows the plan and the budget allow, then waits out what is left. It returns an error only
// when the run was stopped; a focus window that fails turns focusing off for the run and never ends it.
func (f *focuser) until(ctx context.Context, deadline, runEnd time.Time, fp *focusPlan) error {
	step := time.Duration(f.plan.WindowSeconds+focusOverheadSeconds) * time.Second
	for {
		now := f.s.now()
		ch, ok := fp.next()
		if f.off || !ok || deadline.Sub(now) < step || !runEnd.After(now.Add(step)) || !f.fits(now, fp.frames[ch]) {
			return f.waitOut(ctx, deadline)
		}
		if err := f.window(ctx, ch, now); err != nil {
			return err
		}
		f.rec.RefreshMesh(ctx, f.s.now())
	}
}

// waitOut waits until deadline. With the router's list on, it waits in steps of meshAskEvery and asks for the list after each, so the list
// stays fresh through a cycle's idle time; without it, it is one wait.
func (f *focuser) waitOut(ctx context.Context, deadline time.Time) error {
	if !f.rec.hasMesh() {
		return f.s.wait(ctx, max(deadline.Sub(f.s.now()), 0))
	}
	for {
		left := deadline.Sub(f.s.now())
		if left <= 0 {
			return nil
		}
		if err := f.s.wait(ctx, min(left, meshAskEvery)); err != nil {
			return err
		}
		f.rec.RefreshMesh(ctx, f.s.now())
	}
}

// fits says whether one more focus window keeps to the frame budget and to the windows an hour the session was sized for.
func (f *focuser) fits(now time.Time, expected int) bool {
	hours := now.Sub(f.budget.started).Hours()
	return f.budget.allows(now, expected) && float64(f.budget.windows) < hours*float64(f.plan.focusPerHour())+1
}

// window runs one focus window on a channel of the plan, with the plan's own window length, so it is a request the session already approved.
func (f *focuser) window(ctx context.Context, ch int, start time.Time) error {
	p := ListenParams{Iface: f.adapter, Channel: ch, Seconds: f.plan.WindowSeconds}
	res, err := f.priv.Run(ctx, stacksdk.PrivilegedRequest{Action: listenAction, Params: p.request().Params})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	f.budget.windows++
	if err != nil {
		f.stop(messageOf(err))
		return nil
	}
	sum, err := FocusIngest(ctx, f.rec, res.Data, WindowAsked{Channel: ch}, start)
	if err != nil {
		f.stop("a focus window could not be read")
		return nil
	}
	f.budget.frames += sum.Frames
	f.r.update(func(st *ListenState) { st.FocusWindows++; st.FocusFrames += sum.Frames })
	return nil
}

func (f *focuser) stop(why string) {
	f.off = true
	f.r.update(func(st *ListenState) { st.FocusOff, st.LastError = true, why })
}
