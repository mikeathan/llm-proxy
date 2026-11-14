package radio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"
)

// The listening run: the scheduler of plan radio-stack §6 and Constitution I.17. It walks a list of channels, one window each, and repeats on a
// cycle. It starts only when the operator asks (POST ListenPath) and it asks the helper with no grant: the core serves a request only while an open
// session approved exactly that request, so a run with no session is refused on its first window and ends. The run is one goroutine, owned by the
// stack; with none started there is no goroutine at all. It never retries a refusal.
const (
	// ListenPath is the run's state (GET), start (POST) and stop (DELETE).
	ListenPath = "/admin/api/radio/listening"
	// PlanPath answers the exact request list a session must approve for an adapter (GET ?adapter=).
	PlanPath = "/admin/api/radio/listening/plan"

	listenAction = "radio_listen"
	// MaxHours is the longest run a start may ask for: the longest session the core allows.
	MaxHours = 12
	// MaxWindowsPerHour is what the helper's own hourly cap for radio_listen (240) leaves room for.
	MaxWindowsPerHour = 200
	// maxChannels is how many channels a plan may list.
	maxChannels = 16
	// defaultFrameCap is the most frames one run may read before it stops: a run that heard this many is in a place too busy to keep up with.
	defaultFrameCap = 2_000_000
	// maxFailures is how many windows in a row may fail (not be refused) before the run stops.
	maxFailures = 3
	keptWindows = 24
)

// The reasons a run ended, as the page shows them.
const (
	EndedNoSession   = "no_session"   // the helper refused: no open session approves this
	EndedTime        = "time"         // the hours asked for are up
	EndedFrames      = "frames"       // the frame cap was reached
	EndedFailing     = "failing"      // windows kept failing
	EndedStopped     = "stopped"      // the operator stopped it
	EndedSwitchedOff = "switched_off" // the stack was switched off
	EndedStack       = "stack_stopped"
)

// defaultChannels are the plan's six: the three 2.4 GHz channels that carry nearly everything, and the 5 GHz channels of the UK and EU that
// the measured router and its neighbours use. The operator changes them in the settings.
var defaultChannels = []int{1, 6, 11, 36, 60, 149}

// validChannels are the channels the helper's catalogue accepts for a window.
var validChannels = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13,
	36, 40, 44, 48, 52, 56, 60, 64, 100, 104, 108, 112, 116, 120, 124, 128, 132, 136, 140, 149, 153, 157, 161, 165,
}

var adapterName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,14}$`)

// Plan is what the settings amount to once their caps are applied.
type Plan struct {
	Channels      []int `json:"channels"`
	WindowSeconds int   `json:"window_seconds"`
	CycleSeconds  int   `json:"cycle_seconds"`
}

// Plan applies defaults and caps: unknown and repeated channels are dropped, a window is 1 to 30 seconds (20 by default), a cycle is at least 60
// seconds (300 by default), and the channels are trimmed until the windows an hour fit under what the helper allows.
func (s Settings) Plan() Plan {
	p := Plan{WindowSeconds: s.WindowSeconds, CycleSeconds: s.CycleSeconds}
	if p.WindowSeconds <= 0 {
		p.WindowSeconds = 20
	}
	p.WindowSeconds = min(p.WindowSeconds, 30)
	if p.CycleSeconds <= 0 {
		p.CycleSeconds = 300
	}
	p.CycleSeconds = max(p.CycleSeconds, 60)
	want := s.Channels
	if len(want) == 0 {
		want = defaultChannels
	}
	for _, c := range want {
		if slices.Contains(validChannels, c) && !slices.Contains(p.Channels, c) && len(p.Channels) < maxChannels {
			p.Channels = append(p.Channels, c)
		}
	}
	for len(p.Channels) > 1 && len(p.Channels)*3600/p.CycleSeconds > MaxWindowsPerHour {
		p.Channels = p.Channels[:len(p.Channels)-1]
	}
	return p
}

// ListenParams are the parameters of one window, as the catalogue's radio_listen decodes them.
type ListenParams struct {
	Iface   string `json:"iface"`
	Channel int    `json:"channel"`
	Seconds int    `json:"seconds"`
}

// ListenRequest is one entry of the list a session approves.
type ListenRequest struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

func (p ListenParams) request() ListenRequest {
	raw, _ := json.Marshal(p)
	return ListenRequest{Action: listenAction, Params: raw}
}

// Requests is the one list that both the session and the scheduler use: a session approves exactly these, and the run asks exactly these, so
// the two cannot differ.
func (p Plan) Requests(adapter string) []ListenRequest {
	out := make([]ListenRequest, 0, len(p.Channels)+1)
	out = append(out, probeRequest())
	for _, c := range p.Channels {
		out = append(out, ListenParams{Iface: adapter, Channel: c, Seconds: p.WindowSeconds}.request())
	}
	return out
}

// ListenState is what the page shows of the run. It holds counts and words only (Constitution I.16).
type ListenState struct {
	Listening bool            `json:"listening"`
	Adapter   string          `json:"adapter,omitempty"`
	Started   time.Time       `json:"started,omitempty"`
	Until     time.Time       `json:"until,omitempty"`
	Cycles    int             `json:"cycles"`
	Frames    int             `json:"frames"`
	Windows   []WindowSummary `json:"windows"`
	Ended     string          `json:"ended,omitempty"`
	LastError string          `json:"last_error,omitempty"`
	Plan      Plan            `json:"plan"`
}

// PlanView is the answer of PlanPath.
type PlanView struct {
	Adapter  string          `json:"adapter"`
	Plan     Plan            `json:"plan"`
	Requests []ListenRequest `json:"requests"`
	MaxHours int             `json:"max_hours"`
}

type listenBody struct {
	Adapter string  `json:"adapter"`
	Hours   float64 `json:"hours"`
}

// run is the state of the one run.
type run struct {
	mu     sync.Mutex
	state  ListenState
	cancel context.CancelFunc
	done   chan struct{}
}

func (r *run) snapshot() ListenState {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.state
	st.Windows = slices.Clone(st.Windows)
	return st
}

func (r *run) update(f func(*ListenState)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f(&r.state)
}

func jsonResponse(status int, v any) (stacksdk.Response, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInternal, "the answer could not be written")
	}
	return stacksdk.Response{Status: status, ContentType: "application/json", Body: append(body, '\n')}, nil
}

func plainError(status int, msg string) (stacksdk.Response, error) {
	return jsonResponse(status, map[string]string{"error": msg})
}

func (s *Stack) handleListen(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	h, started := s.snapshot()
	if !started {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the radio stack is not running")
	}
	routes := map[string]func() (stacksdk.Response, error){
		http.MethodGet + " " + ScanPath:    func() (stacksdk.Response, error) { return s.handleScan(ctx, h, req) },
		http.MethodPut + " " + ScanPath:    func() (stacksdk.Response, error) { return s.handleScan(ctx, h, req) },
		http.MethodGet + " " + PlanPath:    func() (stacksdk.Response, error) { return s.handlePlan(ctx, h, req) },
		http.MethodGet + " " + ListenPath:  func() (stacksdk.Response, error) { return jsonResponse(http.StatusOK, s.listenState(ctx, h)) },
		http.MethodPost + " " + ListenPath: func() (stacksdk.Response, error) { return s.startListen(ctx, h, req.Body) },
		http.MethodDelete + " " + ListenPath: func() (stacksdk.Response, error) {
			s.endRun(EndedStopped)
			return jsonResponse(http.StatusOK, s.listenState(ctx, h))
		},
	}
	if f, ok := routes[req.Method+" "+req.Path]; ok {
		return f()
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}

func (s *Stack) listenState(ctx context.Context, h stacksdk.Host) ListenState {
	s.mu.Lock()
	r := s.run
	s.mu.Unlock()
	st := ListenState{Windows: []WindowSummary{}}
	if r != nil {
		st = r.snapshot()
	}
	st.Plan = s.settings(ctx, h).Plan()
	return st
}

func (s *Stack) handlePlan(ctx context.Context, h stacksdk.Host, req stacksdk.Request) (stacksdk.Response, error) {
	q, err := url.ParseQuery(req.Query)
	adapter := q.Get("adapter")
	if err != nil || !adapterName.MatchString(adapter) {
		return plainError(http.StatusBadRequest, "name the adapter to listen with")
	}
	plan := s.settings(ctx, h).Plan()
	return jsonResponse(http.StatusOK, PlanView{Adapter: adapter, Plan: plan, Requests: plan.Requests(adapter), MaxHours: MaxHours})
}

func (s *Stack) startListen(ctx context.Context, h stacksdk.Host, body []byte) (stacksdk.Response, error) {
	var b listenBody
	if err := strictDecode(body, &b); err != nil || !adapterName.MatchString(b.Adapter) || b.Hours <= 0 || b.Hours > MaxHours {
		return plainError(http.StatusBadRequest, fmt.Sprintf("name the adapter and a time of more than 0 and at most %d hours", MaxHours))
	}
	set := s.settings(ctx, h)
	switch {
	case !set.Enabled:
		return plainError(http.StatusConflict, "the Radio stack is switched off: switch it on first")
	case h.Privileged == nil || h.Vault == nil:
		return plainError(http.StatusConflict, "the privileged helper is not available to the Radio stack")
	}
	rec, _, err := s.recorder(h)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the radio database could not be opened")
	}
	rec.SetOwnNetworks(set.OwnNetworks)
	if err := rec.RefreshOwnAPs(ctx); err != nil {
		return plainError(http.StatusInternalServerError, "the radio database could not be read")
	}
	plan := set.Plan()
	now := s.now()
	until := now.Add(time.Duration(b.Hours * float64(time.Hour)))

	s.mu.Lock()
	if s.run != nil && s.run.state.Listening {
		s.mu.Unlock()
		return plainError(http.StatusConflict, "it is already listening: stop it first")
	}
	runCtx, cancel := context.WithCancel(context.Background())
	r := &run{cancel: cancel, done: make(chan struct{}), state: ListenState{
		Listening: true, Adapter: b.Adapter, Started: now, Until: until, Windows: []WindowSummary{}, Plan: plan,
	}}
	s.run = r
	s.mu.Unlock()

	go func() {
		defer close(r.done)
		s.listen(runCtx, h.Privileged, rec, r, b.Adapter, plan, until)
	}()
	return jsonResponse(http.StatusAccepted, r.snapshot())
}

func strictDecode(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// endRun stops the run (if there is one) with a reason and waits for its goroutine. It is safe when none is running.
func (s *Stack) endRun(reason string) {
	s.mu.Lock()
	r := s.run
	s.mu.Unlock()
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.state.Listening && r.state.Ended == "" {
		r.state.Ended = reason
	}
	r.mu.Unlock()
	r.cancel()
	<-r.done
	r.update(func(st *ListenState) { st.Listening = false })
}

// listen is the run: windows in order, a wait between cycles, until a reason to stop.
func (s *Stack) listen(ctx context.Context, priv stacksdk.Privileged, rec *Recorder, r *run, adapter string, plan Plan, until time.Time) {
	finish := func(reason string) {
		r.update(func(st *ListenState) {
			st.Listening = false
			if st.Ended == "" {
				st.Ended = reason
			}
		})
	}
	defer func() { rec.DropCycle(); rec.ClearLive() }()
	if err := s.readProbe(ctx, priv, rec.store); err != nil {
		if ctx.Err() != nil {
			finish(EndedStopped)
			return
		}
		r.update(func(st *ListenState) { st.LastError = messageOf(err) })
		finish(EndedNoSession)
		return
	}
	failures := 0
	for {
		rec.BeginCycle()
		for _, ch := range plan.Channels {
			if reason := s.reasonToStop(ctx, r, until); reason != "" {
				finish(reason)
				return
			}
			if reason := s.runWindow(ctx, priv, rec, r, ListenParams{Iface: adapter, Channel: ch, Seconds: plan.WindowSeconds}, &failures); reason != "" {
				finish(reason)
				return
			}
		}
		if err := rec.EndCycle(ctx, s.now()); err != nil {
			r.update(func(st *ListenState) { st.LastError = "a cycle could not be written to the database" })
		}
		r.update(func(st *ListenState) { st.Cycles++ })
		wait := time.Duration(max(plan.CycleSeconds-len(plan.Channels)*plan.WindowSeconds, 5)) * time.Second
		if err := s.wait(ctx, wait); err != nil {
			finish(EndedStopped)
			return
		}
	}
}

// runWindow asks for one window and keeps what it heard. A non-empty answer is why the run ends; empty means go on (a window that failed
// is counted and the next one is tried, up to maxFailures in a row).
func (s *Stack) runWindow(ctx context.Context, priv stacksdk.Privileged, rec *Recorder, r *run, p ListenParams, failures *int) string {
	start := s.now()
	res, err := priv.Run(ctx, stacksdk.PrivilegedRequest{Action: listenAction, Params: p.request().Params})
	if ctx.Err() != nil {
		return EndedStopped
	}
	if err != nil {
		r.update(func(st *ListenState) { st.LastError = messageOf(err) })
		if stacksdk.CodeOf(err) == stacksdk.CodeDenied {
			return EndedNoSession
		}
		*failures++
		if *failures >= maxFailures {
			return EndedFailing
		}
		return ""
	}
	*failures = 0
	sum, ierr := Ingest(ctx, rec, res.Data, WindowAsked{Channel: p.Channel}, start)
	if ierr != nil {
		sum = WindowSummary{Channel: p.Channel, Stop: "unreadable", At: start}
	}
	r.update(func(st *ListenState) {
		st.Frames += sum.Frames
		st.Windows = append(st.Windows, sum)
		if len(st.Windows) > keptWindows {
			st.Windows = st.Windows[len(st.Windows)-keptWindows:]
		}
	})
	return ""
}

func (s *Stack) reasonToStop(ctx context.Context, r *run, until time.Time) string {
	switch {
	case ctx.Err() != nil:
		return EndedStopped
	case !s.now().Before(until):
		return EndedTime
	}
	st := r.snapshot()
	if st.Frames >= s.frameCap() {
		return EndedFrames
	}
	return ""
}

// messageOf is the words of a failure at the port, cut short. A port's message is the core's own sentence, never a device's text.
func messageOf(err error) string {
	var e *stacksdk.Error
	msg := err.Error()
	if errors.As(err, &e) {
		msg = e.Message
	}
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

func jsonBytes(v any) ([]byte, error) { return json.Marshal(v) }
