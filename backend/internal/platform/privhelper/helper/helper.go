// Package helper is the privileged helper's per-connection logic (ADR-0034, Constitution I.11). One process serves one connection and exits:
// no standing privilege. For each connection it decides who is calling before it reads a byte, then applies the three gates, the
// catalogue's checks and its own scope, runs the program under the file jail, and returns the bytes. It never parses what the program
// printed, and it says no more than a code on a refusal. The binary in cmd/llm-proxy-helper is a thin main around this package.
package helper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"llm-proxy/internal/platform/isolation"
	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/wire"
)

const (
	// readTimeout is how long a connected peer has to send its request.
	readTimeout = 5 * time.Second
	// runDirPattern names the per-run working directory the program is jailed to.
	runDirPattern = "llm-proxy-helper-*"
)

// Fixed sentences: a refusal carries a code and one of these, never a detail an attacker could use to probe.
const (
	msgCaller   = "not permitted"
	msgDisabled = "this action is off"
	msgScope    = "the request is outside what this action may do"
	msgInvalid  = "the request is not valid"
	msgFailed   = "the action failed"
)

// Peer is who is on the other end of the connection, as the kernel reports it.
type Peer struct {
	PID int
	UID int
}

// Runner starts a program under the file jail. isolation.Runner satisfies it.
type Runner interface {
	Run(ctx context.Context, p isolation.Program, profile isolation.Profile, args ...string) (isolation.Output, error)
}

// Ambient controls the capability a verified action runs with. Raise puts one capability in the ambient set of the calling thread, so the
// programs started from that thread have it; Clear removes every ambient capability. Both act on the calling thread only.
type Ambient interface {
	Clear() error
	Raise(capability int) error
}

// Config is everything a Helper needs. Each function is a seam for the one thing that cannot run in a test (the kernel's answer about the
// peer, systemd's answer about the main process); production passes the real ones and there is no flag that skips a check.
type Config struct {
	// Class is the privilege class of this unit; an action of another class is refused as unknown.
	Class catalog.Class
	// Actions is the root-owned list of actions this unit may run (HELPER_ACTIONS): the third gate.
	Actions []string
	// Capability is the one capability the actions of this class need while they run (CAP_NET_RAW for the net class); 0 means none.
	Capability int
	// Programs maps a catalogue program name to how it is found.
	Programs map[string]isolation.Program
	// DataDirs are directories a program may read besides its own binary's (nmap's data files); those that do not exist are skipped.
	DataDirs []string
	Runner   Runner
	// Identify reads the peer's identity from the connection.
	Identify func(net.Conn) (Peer, error)
	// MainPID returns the service's main process id from systemd.
	MainPID func(context.Context) (int, error)
	// PtraceScope returns the host's Yama ptrace_scope.
	PtraceScope func() (int, error)
	Ambient     Ambient
	// Log writes one line to the journal.
	Log func(msg string, kv ...any)
}

// Helper serves connections.
type Helper struct {
	cfg Config
}

// New checks the configuration and returns a Helper.
func New(cfg Config) (*Helper, error) {
	switch {
	case cfg.Runner == nil || cfg.Identify == nil || cfg.MainPID == nil || cfg.PtraceScope == nil || cfg.Ambient == nil || cfg.Log == nil:
		return nil, errors.New("helper: every seam is required")
	case cfg.Class != catalog.ClassNet && cfg.Class != catalog.ClassLog:
		return nil, fmt.Errorf("helper: unknown class %q", cfg.Class)
	}
	return &Helper{cfg: cfg}, nil
}

// Serve answers the one request on conn and returns; the caller closes conn. It locks its goroutine to one thread, because capabilities
// and the ambient set belong to a thread and the programs it starts must be forked from the thread that holds them.
func (h *Helper) Serve(ctx context.Context, conn net.Conn) {
	runtime.LockOSThread()
	raised := false
	defer func() {
		if !raised {
			runtime.UnlockOSThread() // a thread that held an ambient capability is left to end with its goroutine
		}
	}()
	if err := h.cfg.Ambient.Clear(); err != nil {
		h.refuse(conn, "", wire.CodeFailed, msgFailed, "clear ambient", err)
		return
	}
	if !h.callerIsTheService(ctx, conn) {
		reply(conn, refusal("", wire.CodeCaller, msgCaller))
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	req, err := wire.ReadRequest(conn)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		reply(conn, refusal("", wire.CodeInvalid, msgInvalid))
		return
	}
	prepared, action, ok := h.admit(conn, req)
	if !ok {
		return
	}
	if h.cfg.Capability != 0 {
		if err := h.cfg.Ambient.Raise(h.cfg.Capability); err != nil {
			h.refuse(conn, req.ID, wire.CodeFailed, msgFailed, "raise capability", err)
			return
		}
		raised = true
	}
	h.run(ctx, conn, req.ID, action, prepared)
}

// callerIsTheService is the control of ADR-0034 item 3: the peer must be the unit's main process. It refuses when it cannot find out.
func (h *Helper) callerIsTheService(ctx context.Context, conn net.Conn) bool {
	scope, err := h.cfg.PtraceScope()
	if err != nil || scope == 0 {
		h.cfg.Log("refused: the host lets a process read another's memory (ptrace_scope), so the caller cannot be told from a child", "ptrace_scope", scope, "error", err)
		return false
	}
	peer, err := h.cfg.Identify(conn)
	if err != nil {
		h.cfg.Log("refused: the peer could not be identified", "error", err)
		return false
	}
	main, err := h.cfg.MainPID(ctx)
	if err != nil || main <= 0 {
		h.cfg.Log("refused: the main process id is not known", "peer_pid", peer.PID, "peer_uid", peer.UID, "error", err)
		return false
	}
	if peer.PID != main {
		h.cfg.Log("refused: the caller is not the service's main process", "peer_pid", peer.PID, "peer_uid", peer.UID, "main_pid", main)
		return false
	}
	return true
}

// admit applies the gates and the checks that need no program: the action must be in this unit's class and list, and its parameters must
// pass the catalogue, whose fixed rules are the helper's whole scope: one private IPv4 host, a fixed port list. The operator's declared
// scan ranges are enforced by the service before it asks (a root-held copy would only be copied from a file the service can write).
func (h *Helper) admit(conn net.Conn, req wire.Request) (catalog.Prepared, catalog.Action, bool) {
	action, err := catalog.Lookup(req.Action)
	if err != nil || action.Class != h.cfg.Class {
		reply(conn, refusal(req.ID, wire.CodeUnknown, msgInvalid))
		return catalog.Prepared{}, catalog.Action{}, false
	}
	if !slices.Contains(h.cfg.Actions, action.ID) {
		reply(conn, refusal(req.ID, wire.CodeDisabled, msgDisabled))
		return catalog.Prepared{}, catalog.Action{}, false
	}
	prepared, err := action.Prepare(req.Params)
	if err != nil {
		code, msg := catalog.Code(err), msgInvalid
		if code == wire.CodeScope {
			msg = msgScope
		}
		reply(conn, refusal(req.ID, code, msg))
		return catalog.Prepared{}, catalog.Action{}, false
	}
	return prepared, action, true
}

// run starts the program under the file jail with the action's caps and replies with what it printed. The run ends when the peer
// disconnects or the action's timeout passes.
func (h *Helper) run(ctx context.Context, conn net.Conn, id string, action catalog.Action, p catalog.Prepared) {
	prog, ok := h.cfg.Programs[p.Program]
	if !ok {
		h.refuse(conn, id, wire.CodeFailed, msgFailed, "no such program", errors.New(p.Program))
		return
	}
	root, err := os.MkdirTemp("", runDirPattern)
	if err != nil {
		h.refuse(conn, id, wire.CodeFailed, msgFailed, "run directory", err)
		return
	}
	defer os.RemoveAll(root)

	ctx, cancel := context.WithTimeout(ctx, action.Caps.Timeout)
	defer cancel()
	go func() {
		var b [1]byte
		_, _ = conn.Read(b[:]) // returns when the peer has gone away
		cancel()
	}()
	profile := isolation.ProfileFor(root)
	profile.Timeout, profile.MaxOutput, profile.NoNetwork = action.Caps.Timeout, action.Caps.MaxOutputBytes, !action.Jail.Network
	profile.ReadPaths = append(h.dataDirs(), ownBinary(prog)...)
	out, err := h.cfg.Runner.Run(ctx, prog, profile, p.Args...)
	switch {
	case errors.Is(err, isolation.ErrTimedOut):
		// What a long scan printed before the timeout is worth returning; the reply says it was cut.
		out.Truncated = true
	case err != nil:
		h.refuse(conn, id, wire.CodeFailed, msgFailed, "program", err)
		return
	}
	reply(conn, wire.Response{V: wire.Version, ID: id, Status: wire.StatusOK, Result: &wire.Result{
		Kind: string(action.Result), Data: out.Stdout, Truncated: out.Truncated,
	}})
}

// ownBinary is the one file to grant for a program given by absolute path. The jail's clean PATH covers only /usr/bin and /bin, so a
// program anywhere else (the helper's own binary, which runs the capture, is in /usr/local/bin) could not be executed once the jail is on.
// Only that file is granted, never its directory. A program found on PATH needs nothing, as PATH's directories are already granted.
func ownBinary(p isolation.Program) []string {
	if filepath.IsAbs(p.Binary) {
		return []string{p.Binary}
	}
	return nil
}

// dataDirs are the readable data directories that exist.
func (h *Helper) dataDirs() []string {
	var dirs []string
	for _, d := range h.cfg.DataDirs {
		if st, err := os.Stat(filepath.Clean(d)); err == nil && st.IsDir() {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// refuse logs the real reason to the journal and sends the peer an error status with a fixed sentence.
func (h *Helper) refuse(conn net.Conn, id string, code wire.Code, msg, what string, err error) {
	h.cfg.Log("failed: "+what, "error", err)
	reply(conn, wire.Response{V: wire.Version, ID: id, Status: wire.StatusError, Code: code, Message: msg})
}

func refusal(id string, code wire.Code, msg string) wire.Response {
	return wire.Response{V: wire.Version, ID: id, Status: wire.StatusRefused, Code: code, Message: msg}
}

func reply(conn net.Conn, r wire.Response) {
	_ = wire.WriteFrame(conn, r, wire.MaxResponseBytes)
}
