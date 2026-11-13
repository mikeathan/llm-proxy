// Command llm-proxy-helper is the privileged helper (ADR-0034, Constitution I.11): a small separate binary that runs one catalogue action for
// the service and exits. systemd starts one process per connection (socket activation, Accept=yes), so no privilege stands around.
// The binary imports only the helper package and what that package allows (an architecture test pins the list).
//
//	llm-proxy-helper --class net --actions scan_udp,os_detect     under its unit: the connected socket is file descriptor 3
//	llm-proxy-helper --class net --actions scan_udp --socket PATH  by hand, as root: listen on PATH, serve one connection, exit
//
// HELPER_ACTIONS (the --actions value) is set by the unit's root-owned drop-in; the service cannot change it. The helper checks each target
// against its own fixed rules (one private IPv4 host); the operator's declared scan ranges are enforced by the service before it asks.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"llm-proxy/internal/platform/isolation"
	"llm-proxy/internal/platform/privhelper/capture"
	"llm-proxy/internal/platform/privhelper/catalog"
	"llm-proxy/internal/platform/privhelper/helper"
	"llm-proxy/internal/platform/privhelper/wire"
)

const (
	captureProgName = "capture"
	hostProgName    = "capture-host"
	defaultUnit     = "llm-proxy.service"
	listenFD        = 3 // SD_LISTEN_FDS_START
	nmapProgName    = "nmap"
	radioProgName   = "radio-listen"
)

// nmapData are where nmap keeps its data files on the supported hosts.
var nmapData = []string{"/usr/share/nmap", "/usr/local/share/nmap"}

var nmap = isolation.Program{
	Name: nmapProgName, Binary: nmapProgName, VersionArgs: []string{"--version"},
	InstallHint: map[string]string{"linux": "sudo apt install nmap   (or: sudo dnf install nmap)"},
}

// The radio class runs iw and ip to read and change an adapter (Constitution I.16); the catalogue builds every argument.
const (
	iwProgName = "iw"
	ipProgName = "ip"
)

var iw = isolation.Program{
	Name: iwProgName, Binary: iwProgName, VersionArgs: []string{"--version"},
	InstallHint: map[string]string{"linux": "sudo apt install iw   (or: sudo dnf install iw)"},
}

var ip = isolation.Program{
	Name: ipProgName, Binary: ipProgName, VersionArgs: []string{"-V"},
	InstallHint: map[string]string{"linux": "sudo apt install iproute2"},
}

func main() {
	// The sandbox runner re-executes this binary to confine a program; that path must run before anything else.
	if isolation.RunChildIfRequested() {
		return
	}
	// The capture_window action runs this same binary as its program, under the file jail and with the one capability the net class holds
	// (Constitution I.11): it is the packet reader, and it serves no connection.
	if len(os.Args) > 1 && os.Args[1] == wire.CaptureCommand {
		os.Exit(capture.Main(os.Args[2:], os.Stdout, os.Stderr))
	}
	// The capture_host action runs this same binary the same way, for the look at this computer's own traffic (Constitution I.13).
	if len(os.Args) > 1 && os.Args[1] == wire.HostCommand {
		os.Exit(capture.HostMain(os.Args[2:], os.Stdout, os.Stderr))
	}
	// The radio_listen action runs this same binary the same way, for the frames a USB Wi-Fi adapter hears (Constitution I.16).
	if len(os.Args) > 1 && os.Args[1] == wire.RadioCommand {
		os.Exit(capture.RadioMain(os.Args[2:], os.Stdout, os.Stderr))
	}
	os.Exit(run(os.Args[1:], os.Environ(), os.Stderr))
}

func run(args []string, env []string, stderr io.Writer) int {
	log := slog.New(slog.NewTextHandler(stderr, nil))
	fs := flag.NewFlagSet("llm-proxy-helper", flag.ContinueOnError)
	fs.SetOutput(stderr)
	class := fs.String("class", "", "privilege class of this unit: net or radio")
	actions := fs.String("actions", "", "comma-separated actions this unit may run")
	socket := fs.String("socket", "", "listen here and serve one connection (by hand); default: the socket systemd passes")
	unit := fs.String("unit", defaultUnit, "the service unit whose main process may call")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	h, err := build(*class, *actions, *unit, log)
	if err != nil {
		log.Error("not started", "error", err)
		return 2
	}
	conn, cleanup, err := connection(*socket, env)
	if err != nil {
		log.Error("no connection", "error", err)
		return 2
	}
	defer cleanup()
	defer conn.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	h.Serve(ctx, conn)
	return 0
}

func build(class, actions, unit string, log *slog.Logger) (*helper.Helper, error) {
	if class != string(catalog.ClassNet) && class != string(catalog.ClassRadio) {
		return nil, fmt.Errorf("--class must be %q or %q (the log class is deferred)", catalog.ClassNet, catalog.ClassRadio)
	}
	list := splitList(actions)
	for _, id := range list {
		if a, err := catalog.Lookup(id); err != nil || string(a.Class) != class {
			return nil, fmt.Errorf("--actions names %q, which is not an action of the %s class", id, class)
		}
	}
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("the helper cannot find its own path for the capture program: %w", err)
	}
	runner := isolation.NewRunner(isolation.New(isolation.Config{Filesystem: true}), nil)
	cfg := helper.Config{
		Class: catalog.ClassNet, Actions: list, Capabilities: []int{helper.CapNetRaw},
		Programs: map[string]isolation.Program{nmapProgName: nmap, captureProgName: {Name: captureProgName, Binary: self}, hostProgName: {Name: hostProgName, Binary: self}}, DataDirs: nmapData,
		Runner: runner, Ambient: helper.ThreadAmbient{},
		Identify: helper.IdentifyPeer, MainPID: helper.SystemdMainPID(runner, unit), PtraceScope: helper.YamaPtraceScope,
		Log: func(msg string, kv ...any) { log.Warn(msg, kv...) },
	}
	if class == string(catalog.ClassRadio) {
		cfg.Class, cfg.Capabilities, cfg.DataDirs = catalog.ClassRadio, []int{helper.CapNetAdmin, helper.CapNetRaw}, nil
		cfg.Programs = map[string]isolation.Program{iwProgName: iw, ipProgName: ip, radioProgName: {Name: radioProgName, Binary: self}}
		cfg.Interface = helper.SystemRadioHost().Check
	}
	return helper.New(cfg)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// connection returns the one connection to serve: the descriptor systemd passed (checked against LISTEN_PID and LISTEN_FDS so a stray
// variable cannot make this process serve something else), or, by hand, the first connection on a socket this process creates.
func connection(socket string, env []string) (net.Conn, func(), error) {
	if socket != "" {
		return acceptOne(socket)
	}
	vars := map[string]string{}
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			vars[k] = v
		}
	}
	if pid, err := strconv.Atoi(vars["LISTEN_PID"]); err != nil || pid != os.Getpid() || vars["LISTEN_FDS"] != "1" {
		return nil, nil, errors.New("not started by socket activation with one descriptor (use --socket to run by hand)")
	}
	f := os.NewFile(listenFD, "activated-connection")
	defer f.Close()
	conn, err := net.FileConn(f)
	if err != nil {
		return nil, nil, fmt.Errorf("the descriptor systemd passed is not a connection: %w", err)
	}
	return conn, func() {}, nil
}

// acceptOne listens on a path, accepts one connection and stops listening. The socket is owned by root and not readable by others, the
// way the unit's socket is; by hand it is for a test as root.
func acceptOne(path string) (net.Conn, func(), error) {
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = ln.Close(); _ = os.Remove(path) }
	if err := os.Chmod(path, 0o660); err != nil {
		cleanup()
		return nil, nil, err
	}
	conn, err := ln.Accept()
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return conn, cleanup, nil
}
