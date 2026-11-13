package helper

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"

	"llm-proxy/internal/platform/isolation"
	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/wire"
)

const (
	listenBody = `{"v":1,"id":"r2","action":"radio_listen","params":{"iface":"wlx1","channel":60,"seconds":20}}`
	probeBody  = `{"v":1,"id":"r3","action":"radio_probe","params":{}}`
	capNetAdm  = 12
)

var upSteps = []string{
	"ip link set dev wlx1 down", "iw dev wlx1 set type monitor", "ip link set dev wlx1 up", "iw dev wlx1 set channel 60", "iw dev wlx1 info",
}
var backSteps = []string{"ip link set dev wlx1 down", "iw dev wlx1 set type managed", "ip link set dev wlx1 up"}

func (r *rig) radioHelper(t *testing.T, actions ...string) *Helper {
	t.Helper()
	h, err := New(Config{
		Class: catalog.ClassRadio, Actions: actions, Capabilities: []int{capNetAdm, CapNetRaw},
		Programs: map[string]isolation.Program{
			"iw": {Name: "iw", Binary: "iw"}, "ip": {Name: "ip", Binary: "ip"}, "radio-listen": {Name: "radio-listen", Binary: "self"},
		},
		Runner: r, Ambient: r,
		Identify:    func(net.Conn) (Peer, error) { return r.peer, nil },
		MainPID:     func(context.Context) (int, error) { return r.mainPID, nil },
		PtraceScope: func() (int, error) { return r.scope, nil },
		Interface: func(_ context.Context, iface string) error {
			r.mu.Lock()
			r.ifaces = append(r.ifaces, iface)
			r.mu.Unlock()
			return r.ifaceErr
		},
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

func TestAListenPutsTheAdapterUpRunsTheProgramAndPutsItBack(t *testing.T) {
	r := newRig()
	r.runOut = isolation.Output{Stdout: []byte("frames")}
	resp := call(t, r.radioHelper(t, "radio_listen"), listenBody)
	if resp.Status != wire.StatusOK || string(resp.Result.Data) != "frames" {
		t.Fatalf("got %+v", resp)
	}
	want := slices.Concat(upSteps, []string{"radio-listen radio-listen --iface wlx1 --seconds 20"}, backSteps)
	if !slices.Equal(r.calls, want) {
		t.Errorf("order of runs:\n got %q\nwant %q", r.calls, want)
	}
	if !slices.Equal(r.raised, []int{capNetAdm, CapNetRaw}) || r.order()[:len("clear,raise,raise,run")] != "clear,raise,raise,run" {
		t.Errorf("both capabilities are raised after the checks and before any run: %v %s", r.raised, r.order())
	}
	if !slices.Equal(r.ifaces, []string{"wlx1"}) {
		t.Errorf("the interface is checked once, before anything runs: %v", r.ifaces)
	}
}

// iw turns an interface name into an index with glibc's if_nametoindex, which opens a Unix socket first. The jail refuses socket(AF_UNIX) to
// every child, so on a real host the jailed `iw dev <name> set type` failed with "No such file or directory (-2)" while `iw list` (no name)
// worked. The fixed ip and iw steps are the one relaxation; the program that listens never makes a Unix socket.
func TestOnlyTheFixedAdapterStepsMayMakeAUnixSocket(t *testing.T) {
	r := newRig()
	r.runOut = isolation.Output{Stdout: []byte("frames")}
	if resp := call(t, r.radioHelper(t, "radio_listen"), listenBody); resp.Status != wire.StatusOK {
		t.Fatalf("got %+v", resp)
	}
	for line, unix := range r.unixOf {
		if want := !strings.HasPrefix(line, "radio-listen "); unix != want {
			t.Errorf("%q: AllowUnixSockets=%v, want %v", line, unix, want)
		}
	}
	if len(r.unixOf) != len(upSteps)+1+len(backSteps)-len(overlap(upSteps, backSteps)) {
		t.Errorf("every distinct run is checked: %v", r.unixOf)
	}
}

func overlap(a, b []string) []string {
	var out []string
	for _, x := range a {
		if slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

func TestTheChannelReadBackIsReturnedInTheReport(t *testing.T) {
	r := newRig()
	r.runOut = isolation.Output{Stdout: []byte("channel 60 (5300 MHz)")}
	resp := call(t, r.radioHelper(t, "radio_listen"), listenBody)
	if resp.Status != wire.StatusOK || string(resp.Result.Report) != "channel 60 (5300 MHz)" {
		t.Fatalf("got %+v", resp.Result)
	}
	probe := call(t, newRig().radioHelper(t, "radio_probe"), probeBody)
	if probe.Status != wire.StatusOK || len(probe.Result.Report) != 0 {
		t.Errorf("a probe has no steps and no report: %+v", probe)
	}
}

func TestAnAdapterIsPutBackWhenASetUpStepFails(t *testing.T) {
	for _, failing := range []string{"set type monitor", "set channel", "info"} {
		r := newRig()
		r.failCall = failing
		resp := call(t, r.radioHelper(t, "radio_listen"), listenBody)
		if resp.Status != wire.StatusError || resp.Code != wire.CodeFailed {
			t.Errorf("%s: got %+v", failing, resp)
		}
		if got := r.calls[len(r.calls)-3:]; !slices.Equal(got, backSteps) {
			t.Errorf("%s: the last three runs must put the adapter back, got %q", failing, got)
		}
		for _, c := range r.calls {
			if strings.HasPrefix(c, "radio-listen") {
				t.Errorf("%s: the program must not run after a failed step", failing)
			}
		}
	}
}

func TestAnAdapterIsPutBackWhenTheProgramFails(t *testing.T) {
	r := newRig()
	r.failCall = "radio-listen"
	resp := call(t, r.radioHelper(t, "radio_listen"), listenBody)
	if resp.Status != wire.StatusError || !slices.Equal(r.calls[len(r.calls)-3:], backSteps) {
		t.Fatalf("got %+v, calls %q", resp, r.calls)
	}
}

func TestEveryStepThatPutsTheAdapterBackRunsEvenWhenOneFails(t *testing.T) {
	r := newRig()
	r.failCall = "set type managed"
	resp := call(t, r.radioHelper(t, "radio_listen"), listenBody)
	if resp.Status != wire.StatusOK {
		t.Errorf("the capture itself succeeded: %+v", resp)
	}
	if got := r.calls[len(r.calls)-3:]; !slices.Equal(got, backSteps) {
		t.Errorf("the step after the failed one still runs: %q", got)
	}
	if !slices.Contains(r.logs, "a step that puts an adapter back failed") {
		t.Errorf("the failure is in the journal: %v", r.logs)
	}
}

func TestAnAdapterIsPutBackWhenThePeerGoesAway(t *testing.T) {
	r := newRig()
	r.blockMain, r.mainRan = true, make(chan struct{})
	h := r.radioHelper(t, "radio_listen")
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() { defer close(done); defer server.Close(); h.Serve(t.Context(), server) }()
	body := []byte(listenBody)
	n := len(body)
	go func() {
		_, _ = client.Write(append([]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}, body...))
	}()
	<-r.mainRan
	client.Close()
	<-done
	r.mu.Lock()
	defer r.mu.Unlock()
	if got := r.calls[len(r.calls)-3:]; !slices.Equal(got, backSteps) {
		t.Errorf("a cancelled window must still put the adapter back, got %q", got)
	}
}

func TestAnInterfaceTheHostRefusesIsRefusedBeforeAnythingRuns(t *testing.T) {
	r := newRig()
	r.ifaceErr = errors.New("carries the default route")
	resp := call(t, r.radioHelper(t, "radio_listen"), listenBody)
	if resp.Status != wire.StatusRefused || resp.Code != wire.CodeScope || strings.Contains(resp.Message, "route") {
		t.Fatalf("got %+v: the peer is told only that it is out of scope", resp)
	}
	if len(r.calls) != 0 || len(r.raised) != 0 {
		t.Errorf("nothing runs and no capability is raised: %q %v", r.calls, r.raised)
	}
	if !slices.Contains(r.logs, "refused: the interface is not one this action may touch") {
		t.Errorf("the reason is in the journal: %v", r.logs)
	}
}

func TestAProbeRunsOneProgramWithNoInterfaceCheck(t *testing.T) {
	r := newRig()
	resp := call(t, r.radioHelper(t, "radio_probe"), probeBody)
	if resp.Status != wire.StatusOK || !slices.Equal(r.calls, []string{"iw list"}) || len(r.ifaces) != 0 {
		t.Fatalf("got %+v, calls %q, ifaces %v", resp, r.calls, r.ifaces)
	}
}

func TestTheRadioClassRefusesNetActionsAndTheNetClassRefusesRadioOnes(t *testing.T) {
	r := newRig()
	if resp := call(t, r.radioHelper(t, "scan_udp"), scanBody); resp.Code != wire.CodeUnknown {
		t.Errorf("a radio unit must not run a net action: %+v", resp)
	}
	if resp := call(t, r.helper(t, "radio_listen"), listenBody); resp.Code != wire.CodeUnknown {
		t.Errorf("a net unit must not run a radio action: %+v", resp)
	}
}

func TestARadioActionThatIsNotInTheDropInIsOff(t *testing.T) {
	r := newRig()
	if resp := call(t, r.radioHelper(t, "radio_probe"), listenBody); resp.Code != wire.CodeDisabled || len(r.calls) != 0 {
		t.Errorf("got %+v, calls %q", resp, r.calls)
	}
}

func TestTheRadioClassNeedsAnInterfaceCheck(t *testing.T) {
	r := newRig()
	_, err := New(Config{
		Class: catalog.ClassRadio, Runner: r, Ambient: r, Identify: func(net.Conn) (Peer, error) { return Peer{}, nil },
		MainPID: func(context.Context) (int, error) { return 1, nil }, PtraceScope: func() (int, error) { return 1, nil }, Log: func(string, ...any) {},
	})
	if err == nil {
		t.Error("a radio unit with no way to check the interface must not start")
	}
}
