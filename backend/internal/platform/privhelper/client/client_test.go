package client

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/helpertest"
	"llm-proxy/internal/platform/privhelper/wire"
	"llm-proxy/internal/platform/sandbox"
)

const scanParams = `{"target":"192.168.1.5","ports":[53,161]}`

type fakeSandbox struct{ state sandbox.EffectiveState }

func (f fakeSandbox) Effective() sandbox.EffectiveState { return f.state }

func enforcing() fakeSandbox {
	on := sandbox.SurfaceState{Mechanism: sandbox.EnforcementLandlock}
	return fakeSandbox{sandbox.EffectiveState{Filesystem: on, UnixSockets: on, Provider: "test"}}
}

func echo(context.Context, catalog.Prepared) ([]byte, error) { return []byte("PORT STATE"), nil }

func TestACallReturnsWhatTheHelperRan(t *testing.T) {
	f := helpertest.New(t, echo)
	res, err := New(f.Path, enforcing()).Call(t.Context(), "scan_udp", json.RawMessage(scanParams))
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Data) != "PORT STATE" || res.Kind != "bytes" {
		t.Errorf("got %+v", res)
	}
	calls := f.Calls()
	if len(calls) != 1 || calls[0].Prepared.Program != "nmap" || calls[0].Prepared.Args[len(calls[0].Prepared.Args)-1] != "192.168.1.5" {
		t.Errorf("the helper saw %+v", calls)
	}
}

func TestNoCallIsMadeUnlessTheSandboxIsEnforcing(t *testing.T) {
	on := sandbox.SurfaceState{Mechanism: sandbox.EnforcementLandlock}
	off := sandbox.SurfaceState{Mechanism: sandbox.EnforcementNone, Reason: "off"}
	cases := map[string]Sandbox{
		"no provider":          nil,
		"no file confinement":  fakeSandbox{sandbox.EffectiveState{Filesystem: off, UnixSockets: on}},
		"unix sockets allowed": fakeSandbox{sandbox.EffectiveState{Filesystem: on, UnixSockets: off}},
		"nothing enforced":     fakeSandbox{sandbox.EffectiveState{Filesystem: off, UnixSockets: off}},
	}
	for name, sb := range cases {
		f := helpertest.New(t, echo)
		_, err := New(f.Path, sb).Call(t.Context(), "scan_udp", json.RawMessage(scanParams))
		if !errors.Is(err, ErrUnsafeHost) {
			t.Errorf("%s: got %v", name, err)
		}
		if n := len(f.Calls()); n != 0 {
			t.Errorf("%s: the helper was called %d times", name, n)
		}
	}
}

func TestABadRequestFailsLocallyWithoutAConnection(t *testing.T) {
	f := helpertest.New(t, echo)
	c := New(f.Path, enforcing())
	cases := map[string]struct {
		action, params string
		want           error
	}{
		"unknown action": {"rm_rf", `{}`, catalog.ErrUnknown},
		"out of scope":   {"os_detect", `{"target":"8.8.8.8"}`, catalog.ErrScope},
		"unknown field":  {"os_detect", `{"target":"192.168.1.5","x":1}`, catalog.ErrInvalid},
		"missing params": {"os_detect", ``, catalog.ErrInvalid},
	}
	for name, tc := range cases {
		if _, err := c.Call(t.Context(), tc.action, json.RawMessage(tc.params)); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if n := len(f.Calls()); n != 0 {
		t.Errorf("the helper was called %d times", n)
	}
}

func TestAMissingHelperIsUnavailable(t *testing.T) {
	dir, err := os.MkdirTemp("", "ph")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	_, err = New(filepath.Join(dir, "none.sock"), enforcing()).Call(t.Context(), "scan_udp", json.RawMessage(scanParams))
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("got %v", err)
	}
}

// answering serves one canned response to the first request, whatever it is, for the answers the real catalogue would not produce.
func answering(t *testing.T, build func(id string) wire.Response) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ph")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "h.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		req, err := wire.ReadRequest(conn)
		if err == nil {
			_ = wire.WriteFrame(conn, build(req.ID), wire.MaxResponseBytes)
		}
	}()
	t.Cleanup(func() { _ = ln.Close(); <-done; os.RemoveAll(dir) })
	return path
}

func TestAHelperRefusalIsATypedRefusal(t *testing.T) {
	path := answering(t, func(id string) wire.Response {
		return wire.Response{V: wire.Version, ID: id, Status: wire.StatusRefused, Code: wire.CodeCaller, Message: "not the service"}
	})
	_, err := New(path, enforcing()).Call(t.Context(), "scan_udp", json.RawMessage(scanParams))
	var r *Refusal
	if !errors.As(err, &r) || r.Code != wire.CodeCaller {
		t.Errorf("got %v", err)
	}
}

func TestAnErrorStatusIsAFailure(t *testing.T) {
	path := answering(t, func(id string) wire.Response {
		return wire.Response{V: wire.Version, ID: id, Status: wire.StatusError, Code: wire.CodeFailed, Message: "the action failed"}
	})
	if _, err := New(path, enforcing()).Call(t.Context(), "scan_udp", json.RawMessage(scanParams)); !errors.Is(err, ErrFailed) {
		t.Errorf("got %v", err)
	}
}

func TestAnAnswerOverTheCapOrOfTheWrongShapeIsRefused(t *testing.T) {
	a, _ := catalog.Lookup("scan_udp")
	cases := map[string]wire.Response{
		"over the output cap": {V: wire.Version, Status: wire.StatusOK, Result: &wire.Result{Kind: "bytes", Data: make([]byte, a.Caps.MaxOutputBytes+1)}},
		"no result":           {V: wire.Version, Status: wire.StatusOK},
		"another kind":        {V: wire.Version, Status: wire.StatusOK, Result: &wire.Result{Kind: "records", Data: []byte("x")}},
	}
	wants := map[string]error{"over the output cap": ErrTooLarge, "no result": wire.ErrMalformed, "another kind": wire.ErrMalformed}
	for name, resp := range cases {
		path := answering(t, func(id string) wire.Response { resp.ID = id; return resp })
		if _, err := New(path, enforcing()).Call(t.Context(), "scan_udp", json.RawMessage(scanParams)); !errors.Is(err, wants[name]) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestAnAnswerToAnotherRequestIsRefused(t *testing.T) {
	path := answering(t, func(string) wire.Response {
		return wire.Response{V: wire.Version, ID: "someone else", Status: wire.StatusOK, Result: &wire.Result{Kind: "bytes", Data: []byte("x")}}
	})
	if _, err := New(path, enforcing()).Call(t.Context(), "scan_udp", json.RawMessage(scanParams)); !errors.Is(err, wire.ErrMalformed) {
		t.Errorf("got %v", err)
	}
}

// Cancelling the call closes the connection, and the helper cancels its run when the peer goes away (design section 6).
func TestCancellingACallStopsTheHelpersRun(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	f := helpertest.New(t, func(ctx context.Context, _ catalog.Prepared) ([]byte, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	})
	ctx, cancel := context.WithCancel(t.Context())
	errc := make(chan error, 1)
	go func() {
		_, err := New(f.Path, enforcing()).Call(ctx, "scan_udp", json.RawMessage(scanParams))
		errc <- err
	}()
	<-started
	cancel()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Errorf("the caller must see its own cancellation, got %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the helper's run was not cancelled when the caller went away")
	}
}

// Against an installed helper (scripts/check-helper-under-unit.sh's setup): run from the service's main process with HELPER_SOCKET and
// HELPER_TARGET set; skipped everywhere else. It proves the Go client and the real helper speak the same protocol.
func TestAgainstARealHelper(t *testing.T) {
	socket, target := os.Getenv("HELPER_SOCKET"), os.Getenv("HELPER_TARGET")
	if socket == "" || target == "" {
		t.Skip("set HELPER_SOCKET and HELPER_TARGET to run against an installed helper")
	}
	params := json.RawMessage(`{"target":"` + target + `","ports":[53,161]}`)
	res, err := New(socket, enforcing()).Call(t.Context(), "scan_udp", params)
	if err != nil {
		t.Fatalf("a scan through the real helper: %v", err)
	}
	if res.Kind != "bytes" || !strings.Contains(string(res.Data), "Nmap scan report for "+target) {
		t.Errorf("the answer is not nmap's report: %q", res.Data)
	}
	_, err = New(socket, enforcing()).Call(t.Context(), "scan_udp", json.RawMessage(`{"target":"192.168.77.5","ports":[53]}`))
	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Code != wire.CodeScope {
		t.Errorf("a target outside the helper's declared range must be a scope refusal, got %v", err)
	}
}
