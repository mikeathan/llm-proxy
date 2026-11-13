package helper

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/platform/isolation"
	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/wire"
)

const (
	servicePID = 4242
	scanBody   = `{"v":1,"id":"r1","action":"scan_udp","params":{"target":"192.168.50.10","ports":[53,161]}}`
)

// rig is a Helper whose seams are fakes that record what the helper did and in what order.
type rig struct {
	mu       sync.Mutex
	events   []string
	peer     Peer
	peerErr  error
	mainPID  int
	mainErr  error
	scope    int
	scopeErr error
	runOut   isolation.Output
	runErr   error
	ran      chan struct{}
	gotArgs  []string
	gotProf  isolation.Profile
	gotProg  string
	logs     []string
	capRaise int
	raised   []int
	// calls is every program run, as "name args", in order; failCall makes the run whose line contains it fail; blockMain makes the program
	// named radio-listen wait until its context ends.
	calls     []string
	unixOf    map[string]bool // AllowUnixSockets of each run, keyed by its "name args" line
	failCall  string
	blockMain bool
	mainRan   chan struct{}
	ifaceErr  error
	ifaces    []string
}

func (r *rig) note(s string) {
	r.mu.Lock()
	r.events = append(r.events, s)
	r.mu.Unlock()
}

func (r *rig) Clear() error { r.note("clear"); return nil }
func (r *rig) Raise(c int) error {
	r.note("raise")
	r.capRaise = c
	r.raised = append(r.raised, c)
	return nil
}
func (r *rig) Run(ctx context.Context, p isolation.Program, prof isolation.Profile, args ...string) (isolation.Output, error) {
	r.note("run")
	r.gotProg, r.gotProf, r.gotArgs = p.Name, prof, args
	line := p.Name + " " + strings.Join(args, " ")
	r.mu.Lock()
	r.calls = append(r.calls, line)
	if r.unixOf == nil {
		r.unixOf = map[string]bool{}
	}
	r.unixOf[line] = prof.AllowUnixSockets
	r.mu.Unlock()
	if r.failCall != "" && strings.Contains(line, r.failCall) {
		return isolation.Output{}, errors.New("step failed")
	}
	if r.blockMain && p.Name == "radio-listen" {
		close(r.mainRan)
		<-ctx.Done()
		return isolation.Output{}, ctx.Err()
	}
	if r.ran != nil {
		close(r.ran)
		<-ctx.Done()
		return isolation.Output{}, ctx.Err()
	}
	return r.runOut, r.runErr
}

func newRig() *rig {
	return &rig{peer: Peer{PID: servicePID, UID: 995}, mainPID: servicePID, scope: 1, runOut: isolation.Output{Stdout: []byte("PORT STATE")}}
}

func (r *rig) helper(t *testing.T, actions ...string) *Helper {
	t.Helper()
	h, err := New(Config{
		Class: catalog.ClassNet, Actions: actions, Capabilities: []int{CapNetRaw},
		Programs: map[string]isolation.Program{"nmap": {Name: "nmap", Binary: "nmap"}},
		DataDirs: []string{"/definitely/not/a/dir", t.TempDir()},
		Runner:   r, Ambient: r,
		Identify:    func(net.Conn) (Peer, error) { return r.peer, r.peerErr },
		MainPID:     func(context.Context) (int, error) { return r.mainPID, r.mainErr },
		PtraceScope: func() (int, error) { return r.scope, r.scopeErr },
		Log: func(msg string, kv ...any) {
			r.mu.Lock()
			r.logs = append(r.logs, msg)
			r.mu.Unlock()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// call connects a client, sends body as one frame and returns the response Serve wrote.
func call(t *testing.T, h *Helper, body string) wire.Response {
	t.Helper()
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() { defer close(done); defer server.Close(); h.Serve(t.Context(), server) }()
	go func() {
		n := len(body)
		_, _ = client.Write(append([]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}, body...))
	}()
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	var r wire.Response
	err := wire.ReadFrame(client, wire.MaxResponseBytes, &r)
	client.Close()
	<-done
	if err != nil {
		t.Fatalf("no response: %v", err)
	}
	return r
}

func (r *rig) order() string { r.mu.Lock(); defer r.mu.Unlock(); return strings.Join(r.events, ",") }

func TestAVerifiedRequestRunsTheProgramJailedAndReturnsItsBytes(t *testing.T) {
	r := newRig()
	resp := call(t, r.helper(t, "scan_udp"), scanBody)
	if resp.Status != wire.StatusOK || resp.ID != "r1" || string(resp.Result.Data) != "PORT STATE" {
		t.Fatalf("got %+v", resp)
	}
	if r.gotProg != "nmap" || r.gotArgs[len(r.gotArgs)-1] != "192.168.50.10" || !slices.Contains(r.gotArgs, "53,161") {
		t.Errorf("ran %s %v", r.gotProg, r.gotArgs)
	}
	a, _ := catalog.Lookup("scan_udp")
	if r.gotProf.Timeout != a.Caps.Timeout || r.gotProf.MaxOutput != a.Caps.MaxOutputBytes || r.gotProf.NoNetwork || r.gotProf.AllowUnixSockets {
		t.Errorf("the profile must carry the action's caps, keep the network a scan needs, and refuse Unix sockets: %+v", r.gotProf)
	}
	if len(r.gotProf.ReadPaths) != 1 {
		t.Errorf("only data directories that exist may be granted: %v", r.gotProf.ReadPaths)
	}
	if r.order() != "clear,raise,run" || r.capRaise != CapNetRaw {
		t.Errorf("the ambient set is cleared first, and the capability is raised only after every check and before the run: %s", r.order())
	}
}

func TestACallerThatIsNotTheMainProcessIsRefusedBeforeItsRequestIsRead(t *testing.T) {
	cases := map[string]func(*rig){
		"another process":         func(r *rig) { r.peer.PID = servicePID + 1 },
		"the same uid and cgroup": func(r *rig) { r.peer = Peer{PID: 5000, UID: 995} },
		"main pid unknown":        func(r *rig) { r.mainErr = errors.New("systemd said no") },
		"main pid zero":           func(r *rig) { r.mainPID = 0 },
		"peer not identifiable":   func(r *rig) { r.peerErr = errors.New("no creds") },
		"ptrace_scope 0":          func(r *rig) { r.scope = 0 },
		"ptrace_scope unreadable": func(r *rig) { r.scopeErr = errors.New("no yama") },
	}
	for name, mutate := range cases {
		r := newRig()
		mutate(r)
		resp := call(t, r.helper(t, "scan_udp"), scanBody)
		if resp.Status != wire.StatusRefused || resp.Code != wire.CodeCaller || resp.ID != "" || resp.Result != nil {
			t.Errorf("%s: got %+v", name, resp)
		}
		if strings.Contains(r.order(), "run") || strings.Contains(r.order(), "raise") {
			t.Errorf("%s: nothing may run or gain a capability for a refused caller: %s", name, r.order())
		}
		if len(r.logs) == 0 {
			t.Errorf("%s: the refusal must reach the journal", name)
		}
	}
}

func TestAnActionThatIsNotAllowedHereIsRefused(t *testing.T) {
	cases := map[string]struct {
		actions []string
		body    string
		code    wire.Code
	}{
		"not in the root-owned list":  {[]string{"os_detect"}, scanBody, wire.CodeDisabled},
		"an empty list":               {nil, scanBody, wire.CodeDisabled},
		"an unknown action":           {[]string{"scan_udp"}, `{"v":1,"id":"r1","action":"rm_rf","params":{}}`, wire.CodeUnknown},
		"outside the catalogue scope": {[]string{"scan_udp"}, `{"v":1,"id":"r1","action":"scan_udp","params":{"target":"8.8.8.8","ports":[53]}}`, wire.CodeScope},
		"malformed parameters":        {[]string{"scan_udp"}, `{"v":1,"id":"r1","action":"scan_udp","params":{"target":"192.168.50.10","ports":[53],"x":1}}`, wire.CodeInvalid},
		"not a valid request":         {[]string{"scan_udp"}, `hello`, wire.CodeInvalid},
	}
	for name, c := range cases {
		r := newRig()
		resp := call(t, r.helper(t, c.actions...), c.body)
		if resp.Status != wire.StatusRefused || resp.Code != c.code {
			t.Errorf("%s: got %+v, want refused %s", name, resp, c.code)
		}
		if strings.Contains(r.order(), "run") || strings.Contains(r.order(), "raise") {
			t.Errorf("%s: nothing may run or gain a capability: %s", name, r.order())
		}
	}
}

// The helper's own scope is the catalogue's fixed rule (one private IPv4 host). The operator's declared ranges are enforced by the service
// before it asks, so the helper holds no range of its own: a root-held copy would only be copied from a file the service can write.
func TestAnyPrivateHostIsAcceptedBecauseTheServiceEnforcesTheDeclaredRanges(t *testing.T) {
	r := newRig()
	for _, target := range []string{"192.168.1.5", "10.9.9.9", "172.20.0.4"} {
		body := `{"v":1,"id":"r1","action":"scan_udp","params":{"target":"` + target + `","ports":[53]}}`
		if resp := call(t, r.helper(t, "scan_udp"), body); resp.Status != wire.StatusOK {
			t.Errorf("%s: %+v", target, resp)
		}
	}
}

func TestARefusalNeverEchoesTheRequest(t *testing.T) {
	r := newRig()
	resp := call(t, r.helper(t, "scan_udp"), `{"v":1,"id":"r1","action":"scan_udp","params":{"target":"8.8.8.8","ports":[53]}}`)
	if strings.Contains(resp.Message, "8.8.8.8") || strings.Contains(resp.Message, "scan_udp") {
		t.Errorf("the message must be a fixed sentence: %q", resp.Message)
	}
}

func TestAFailedRunSaysOnlyThatItFailedAndLogsWhy(t *testing.T) {
	r := newRig()
	r.runErr = errors.New("nmap: /usr/share/nmap/secret path exploded")
	resp := call(t, r.helper(t, "scan_udp"), scanBody)
	if resp.Status != wire.StatusError || resp.Code != wire.CodeFailed || strings.Contains(resp.Message, "secret") {
		t.Errorf("got %+v", resp)
	}
	if len(r.logs) == 0 {
		t.Error("the reason must reach the journal")
	}
}

func TestATimedOutScanReturnsWhatItPrintedMarkedCut(t *testing.T) {
	r := newRig()
	r.runErr = isolation.ErrTimedOut
	r.runOut = isolation.Output{Stdout: []byte("partial")}
	resp := call(t, r.helper(t, "scan_udp"), scanBody)
	if resp.Status != wire.StatusOK || string(resp.Result.Data) != "partial" || !resp.Result.Truncated {
		t.Errorf("got %+v", resp)
	}
}

func TestTheRunEndsWhenThePeerGoesAway(t *testing.T) {
	r := newRig()
	r.ran = make(chan struct{})
	h := r.helper(t, "scan_udp")
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() { defer close(done); defer server.Close(); h.Serve(t.Context(), server) }()
	n := len(scanBody)
	go client.Write(append([]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}, scanBody...))
	select {
	case <-r.ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the program never started")
	}
	client.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the helper kept running after the peer left")
	}
}

func TestNewRefusesAnIncompleteConfiguration(t *testing.T) {
	if _, err := New(Config{Class: catalog.ClassNet}); err == nil {
		t.Error("a helper with no seams must be refused")
	}
	r := newRig()
	h := r.helper(t)
	cfg := h.cfg
	cfg.Class = "root"
	if _, err := New(cfg); err == nil {
		t.Error("an unknown class must be refused")
	}
}

func TestALogClassUnitRefusesNetActions(t *testing.T) {
	r := newRig()
	h := r.helper(t, "scan_udp")
	h.cfg.Class = catalog.ClassLog
	if resp := call(t, h, scanBody); resp.Code != wire.CodeUnknown {
		t.Errorf("an action of another class must not run in this unit: %+v", resp)
	}
}
