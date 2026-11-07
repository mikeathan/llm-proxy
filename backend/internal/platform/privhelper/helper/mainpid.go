package helper

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"llm-proxy/internal/platform/isolation"
)

const (
	mainPIDTimeout   = 5 * time.Second
	mainPIDMaxOutput = 64
)

// systemctlProgram is how systemctl is found. It is the only program the helper starts that is allowed Unix sockets (it talks to
// systemd over the system bus), so it runs with no network and, because the helper clears the ambient set before asking, no capability.
var systemctlProgram = isolation.Program{
	Name: "systemctl", Binary: "systemctl", VersionArgs: []string{"--version"},
	InstallHint: map[string]string{"linux": "systemd provides systemctl"},
}

// SystemdMainPID asks systemd for the unit's main process id with a fixed, read-only query, run through the file jail with Unix sockets
// as the one relaxation (design decision D3; the precedent is nmcli, ADR-0031).
func SystemdMainPID(r Runner, unit string) func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		root, err := os.MkdirTemp("", runDirPattern)
		if err != nil {
			return 0, fmt.Errorf("main pid: %w", err)
		}
		defer os.RemoveAll(root)
		profile := isolation.ProfileFor(root)
		profile.AllowUnixSockets, profile.NoNetwork = true, true
		profile.Timeout, profile.MaxOutput = mainPIDTimeout, mainPIDMaxOutput
		out, err := r.Run(ctx, systemctlProgram, profile, "show", "-p", "MainPID", "--value", unit)
		if err != nil {
			return 0, fmt.Errorf("main pid: %w", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(out.Stdout)))
		if err != nil || pid <= 0 {
			return 0, fmt.Errorf("main pid: systemd reported %q", strings.TrimSpace(string(out.Stdout)))
		}
		return pid, nil
	}
}
