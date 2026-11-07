// Package helpertest is a fake privileged helper for tests, and the wire contract every helper must pass (design section 10, test 4). It
// speaks the real wire protocol over a real Unix socket and applies the real catalogue, but starts no program and holds no privilege. It
// is imported by tests only; an architecture test keeps it out of production code.
package helpertest

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/wire"
)

// readTimeout is how long the fake waits for a request.
const readTimeout = 5 * time.Second

// RunFunc stands in for starting the program: it receives the prepared action and returns what the program would print. It must
// return when ctx ends.
type RunFunc func(ctx context.Context, p catalog.Prepared) ([]byte, error)

// Call is one request the fake answered.
type Call struct {
	Action   string
	Prepared catalog.Prepared
}

// Fake is a helper listening on a Unix socket in a test's temporary directory.
type Fake struct {
	Path string

	run   RunFunc
	ln    net.Listener
	wg    sync.WaitGroup
	mu    sync.Mutex
	calls []Call
}

// New starts a fake helper that answers every valid request with run's output. It stops with the test.
func New(t *testing.T, run RunFunc) *Fake {
	t.Helper()
	// t.TempDir embeds the test's name, which can push a Unix socket path past the 104 bytes macOS allows.
	dir, err := os.MkdirTemp("", "ph")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	f := &Fake{Path: filepath.Join(dir, "h.sock"), run: run}
	ln, err := net.Listen("unix", f.Path)
	if err != nil {
		t.Fatal(err)
	}
	f.ln = ln
	f.wg.Go(f.accept)
	t.Cleanup(func() { _ = ln.Close(); f.wg.Wait() })
	return f
}

// Calls returns the requests answered so far.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

func (f *Fake) accept() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.wg.Go(func() { f.serve(conn) })
	}
}

// serve answers exactly one request and closes: a second request on the connection finds it closed.
func (f *Fake) serve(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	req, err := wire.ReadRequest(conn)
	if err != nil {
		reply(conn, wire.Response{V: wire.Version, Status: wire.StatusRefused, Code: wire.CodeInvalid, Message: "the request is not valid"})
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	a, err := catalog.Lookup(req.Action)
	if err != nil {
		reply(conn, refusal(req.ID, wire.CodeUnknown, "no such action"))
		return
	}
	prepared, err := a.Prepare(req.Params)
	if err != nil {
		reply(conn, refusal(req.ID, catalog.Code(err), "the parameters are refused"))
		return
	}
	f.mu.Lock()
	f.calls = append(f.calls, Call{Action: a.ID, Prepared: prepared})
	f.mu.Unlock()

	// The run ends when the action's timeout does or when the peer goes away: a read on a connection whose peer has closed returns.
	ctx, cancel := context.WithTimeout(context.Background(), a.Caps.Timeout)
	defer cancel()
	go func() {
		var b [1]byte
		_, _ = conn.Read(b[:])
		cancel()
	}()
	out, err := f.run(ctx, prepared)
	if err != nil {
		code := wire.CodeFailed
		if errors.Is(err, context.DeadlineExceeded) {
			code = wire.CodeCaps
		}
		reply(conn, wire.Response{V: wire.Version, ID: req.ID, Status: wire.StatusError, Code: code, Message: "the action failed"})
		return
	}
	res := wire.Result{Kind: string(a.Result), Data: out}
	if len(out) > a.Caps.MaxOutputBytes {
		res.Data, res.Truncated = out[:a.Caps.MaxOutputBytes], true
	}
	reply(conn, wire.Response{V: wire.Version, ID: req.ID, Status: wire.StatusOK, Result: &res})
}

func refusal(id string, code wire.Code, msg string) wire.Response {
	return wire.Response{V: wire.Version, ID: id, Status: wire.StatusRefused, Code: code, Message: msg}
}

func reply(conn net.Conn, r wire.Response) {
	_ = wire.WriteFrame(conn, r, wire.MaxResponseBytes)
}

// Sample is one request every helper must answer with status ok; the contract sends it to prove the success path.
type Sample struct {
	Action string
	Params string
}

// Contract is the wire contract every helper passes, the real one and this fake (design section 10, test 4): a valid request is
// answered; a request that breaks a rule of the wire or the catalogue is refused with a code and no detail; and a connection carries one
// request, so a second one gets no answer.
func Contract(t *testing.T, socket string, ok Sample) {
	t.Helper()
	valid := func(extra string) string {
		return `{"v":1,"id":"c1","action":"` + ok.Action + `","params":` + ok.Params + extra + `}`
	}
	t.Run("a valid request is answered ok", func(t *testing.T) {
		r := exchange(t, socket, valid(""))
		if r.Status != wire.StatusOK || r.ID != "c1" || r.Result == nil || len(r.Result.Data) == 0 {
			t.Errorf("got %+v", r)
		}
	})
	contractRefusals(t, socket, ok, valid)
	t.Run("refuses a frame longer than the cap without reading it", func(t *testing.T) {
		conn := dial(t, socket)
		defer conn.Close()
		_, _ = conn.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
		var r wire.Response
		if err := wire.ReadFrame(conn, wire.MaxResponseBytes, &r); err != nil || r.Status != wire.StatusRefused {
			t.Errorf("got %+v, %v", r, err)
		}
	})
	t.Run("one request per connection", func(t *testing.T) {
		conn := dial(t, socket)
		defer conn.Close()
		writeRaw(t, conn, valid(""))
		var r wire.Response
		if err := wire.ReadFrame(conn, wire.MaxResponseBytes, &r); err != nil || r.Status != wire.StatusOK {
			t.Fatalf("first request: %+v, %v", r, err)
		}
		_ = wire.WriteFrame(conn, wire.Request{V: wire.Version, ID: "c2", Action: ok.Action, Params: []byte(ok.Params)}, wire.MaxRequestBytes)
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		if err := wire.ReadFrame(conn, wire.MaxResponseBytes, &r); err == nil {
			t.Errorf("a second request on one connection was answered: %+v", r)
		}
	})
}

// contractRefusals sends requests that break one rule each and expects a refusal with the right code and no result.
func contractRefusals(t *testing.T, socket string, ok Sample, valid func(extra string) string) {
	refused := map[string]struct {
		frame string
		code  wire.Code
	}{
		"an unknown field":         {valid(`,"extra":1`), wire.CodeInvalid},
		"a wrong version":          {`{"v":2,"id":"c1","action":"` + ok.Action + `","params":` + ok.Params + `}`, wire.CodeInvalid},
		"data after the document":  {valid("") + `{}`, wire.CodeInvalid},
		"not json":                 {`hello`, wire.CodeInvalid},
		"an unknown action":        {`{"v":1,"id":"c1","action":"rm_rf","params":{}}`, wire.CodeUnknown},
		"parameters out of scope":  {`{"v":1,"id":"c1","action":"os_detect","params":{"target":"8.8.8.8"}}`, wire.CodeScope},
		"parameters with a field":  {`{"v":1,"id":"c1","action":"os_detect","params":{"target":"192.168.1.5","script":"vuln"}}`, wire.CodeInvalid},
		"parameters that are text": {`{"v":1,"id":"c1","action":"os_detect","params":"192.168.1.5"}`, wire.CodeInvalid},
	}
	for name, c := range refused {
		t.Run("refuses "+name, func(t *testing.T) {
			r := exchange(t, socket, c.frame)
			if r.Status != wire.StatusRefused || r.Code != c.code || r.Result != nil {
				t.Errorf("got %+v, want refused %s", r, c.code)
			}
		})
	}
}

func dial(t *testing.T, socket string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("unix", socket, readTimeout)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(readTimeout))
	return conn
}

// writeRaw sends body as one frame without going through the wire package's encoder, so a test can send what the encoder would not.
func writeRaw(t *testing.T, conn net.Conn, body string) {
	t.Helper()
	n := len(body)
	if _, err := conn.Write(append([]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}, body...)); err != nil {
		t.Fatal(err)
	}
}

func exchange(t *testing.T, socket, body string) wire.Response {
	t.Helper()
	conn := dial(t, socket)
	defer conn.Close()
	writeRaw(t, conn, body)
	var r wire.Response
	if err := wire.ReadFrame(conn, wire.MaxResponseBytes, &r); err != nil {
		t.Fatalf("no answer to %q: %v", body, err)
	}
	return r
}
