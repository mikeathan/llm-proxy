package providers

import (
	"errors"
	"slices"
	"strconv"
	"testing"

	"llm-proxy/internal/platform/process"
	"llm-proxy/models"
)

// fakeReclaimer wires a portReclaimer to an in-memory port and process table.
type fakeReclaimer struct {
	busy      bool
	procs     []process.Info
	listErr   error
	killedPID []int
	listedFor string
}

func (f *fakeReclaimer) build() portReclaimer {
	return portReclaimer{
		inUse: func(int) bool { return f.busy },
		list: func(binary string, _ int) ([]process.Info, error) {
			f.listedFor = binary
			return f.procs, f.listErr
		},
		kill: func(pid int) error {
			f.killedPID = append(f.killedPID, pid)
			f.busy = false
			return nil
		},
	}
}

// Only an orphan of our own server binary on this exact port may be killed; any
// other listener fails the start without being touched.
func TestPortReclaimer(t *testing.T) {
	const port = 8081
	ours := process.Info{PID: 11, Binary: "/opt/llama/llama-server", Port: port}
	otherPort := process.Info{PID: 12, Binary: "/opt/llama/llama-server", Port: 9999}

	for _, tc := range []struct {
		name     string
		fake     fakeReclaimer
		wantErr  bool
		wantKill []int
	}{
		{"free port is left alone", fakeReclaimer{}, false, nil},
		{"orphan of our binary on the port is reclaimed", fakeReclaimer{busy: true, procs: []process.Info{ours}}, false, []int{11}},
		{"our binary on another port is not touched", fakeReclaimer{busy: true, procs: []process.Info{otherPort}}, true, nil},
		{"foreign listener (no matching process) is not touched", fakeReclaimer{busy: true}, true, nil},
		{"process listing failure fails closed", fakeReclaimer{busy: true, listErr: errors.New("ps failed")}, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fake.build().reclaim(port, "/opt/llama/llama-server")
			if (err != nil) != tc.wantErr {
				t.Fatalf("reclaim error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrPortInUse) {
				t.Errorf("error %v does not wrap ErrPortInUse", err)
			}
			if len(tc.fake.killedPID) != len(tc.wantKill) || (len(tc.wantKill) > 0 && tc.fake.killedPID[0] != tc.wantKill[0]) {
				t.Errorf("killed = %v, want %v", tc.fake.killedPID, tc.wantKill)
			}
		})
	}
}

func TestPortReclaimer_MatchesOnBinaryBaseName(t *testing.T) {
	f := &fakeReclaimer{busy: true, procs: []process.Info{{PID: 1, Port: 8081}}}
	if err := f.build().reclaim(8081, "/opt/llama/bin/llama-server"); err != nil {
		t.Fatal(err)
	}
	if f.listedFor != "llama-server" {
		t.Errorf("listed processes for %q, want the binary base name", f.listedFor)
	}
}

// A kill that leaves the port bound must not report success.
func TestPortReclaimer_StillBusyAfterKillFails(t *testing.T) {
	f := &fakeReclaimer{busy: true, procs: []process.Info{{PID: 1, Port: 8081}}}
	r := f.build()
	r.kill = func(int) error { return nil } // does not release the port
	if err := r.reclaim(8081, "llama-server"); !errors.Is(err, ErrPortInUse) {
		t.Fatalf("reclaim error = %v, want ErrPortInUse", err)
	}
}

// Orphan recovery matches processes by the "--port <n>" pair that
// process.parseArgs reads back from ps//proc. If the launch args ever change
// shape, our own orphans would stop being recognised and every restart after a
// crash would fail with ErrPortInUse.
func TestBuildLaunchArgs_PortShapeMatchesOrphanDetection(t *testing.T) {
	args := BuildLaunchArgs(models.ModelConfig{Name: "m", Path: "/models/m.gguf", Port: 8081}, "127.0.0.1")
	i := slices.Index(args, "--port")
	if i < 0 || i+1 >= len(args) || args[i+1] != strconv.Itoa(8081) {
		t.Fatalf("launch args %v must contain the pair \"--port\", \"8081\"", args)
	}
}
