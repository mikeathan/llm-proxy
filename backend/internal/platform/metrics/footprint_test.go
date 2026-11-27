package metrics

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeSampler struct {
	samples map[int32]ProcSample
	err     map[int32]error
	calls   int
	retain  []int32
}

func (f *fakeSampler) Sample(pid int32) (ProcSample, error) {
	f.calls++
	if err := f.err[pid]; err != nil {
		return ProcSample{}, err
	}
	return f.samples[pid], nil
}

func (f *fakeSampler) Retain(pids []int32) { f.retain = pids }

type fakeGPU struct{ mem map[int32]ProcGPUMem }

func (f fakeGPU) ProcMem(pid int32) (ProcGPUMem, bool) { m, ok := f.mem[pid]; return m, ok }

func newTestReader(s *fakeSampler, g ProcGPUReader, clock *time.Time) *FootprintReader {
	r := NewFootprintReader(s, g)
	r.nowFn = func() time.Time { return *clock }
	return r
}

func TestFootprintCPUIsADeltaBetweenReads(t *testing.T) {
	clock := time.Unix(1000, 0)
	created := time.Unix(900, 0)
	s := &fakeSampler{samples: map[int32]ProcSample{7: {CPUSeconds: 10, RSSBytes: 512 << 20, Threads: 12, OpenFiles: 30, CreatedAt: created}}}
	r := newTestReader(s, nil, &clock)
	targets := []FootprintTarget{{Role: RoleService, PID: 7}}

	first := r.Read(targets)
	if got := first.Processes[0].CPUPercent; got != nil {
		t.Fatalf("first read has no baseline, want nil CPU, got %v", *got)
	}
	if first.Processes[0].RSSMB != 512 || first.Processes[0].Threads != 12 {
		t.Fatalf("memory/threads wrong: %+v", first.Processes[0])
	}
	if first.Processes[0].UptimeSec != 100 {
		t.Fatalf("uptime = %v, want 100", first.Processes[0].UptimeSec)
	}

	clock = clock.Add(5 * time.Second)
	s.samples[7] = ProcSample{CPUSeconds: 12.5, RSSBytes: 512 << 20, Threads: 12, OpenFiles: 30, CreatedAt: created}
	second := r.Read(targets)
	got := second.Processes[0].CPUPercent
	if got == nil || *got < 49.9 || *got > 50.1 {
		t.Fatalf("CPU = %v, want 50 (2.5 cpu-seconds over 5 s)", got)
	}
}

func TestFootprintCachesWithinTheWindow(t *testing.T) {
	clock := time.Unix(1000, 0)
	s := &fakeSampler{samples: map[int32]ProcSample{7: {CreatedAt: time.Unix(1, 0)}}}
	r := newTestReader(s, nil, &clock)
	targets := []FootprintTarget{{Role: RoleService, PID: 7}}
	r.Read(targets)
	clock = clock.Add(footprintCacheInterval / 2)
	r.Read(targets)
	if s.calls != 1 {
		t.Fatalf("sampler called %d times inside the cache window, want 1", s.calls)
	}
	clock = clock.Add(footprintCacheInterval)
	r.Read(targets)
	if s.calls != 2 {
		t.Fatalf("sampler called %d times after the window, want 2", s.calls)
	}
}

func TestFootprintNewTargetsBypassTheCache(t *testing.T) {
	clock := time.Unix(1000, 0)
	s := &fakeSampler{samples: map[int32]ProcSample{7: {CreatedAt: time.Unix(1, 0)}, 8: {CreatedAt: time.Unix(1, 0)}}}
	r := newTestReader(s, nil, &clock)
	r.Read([]FootprintTarget{{Role: RoleService, PID: 7}})
	out := r.Read([]FootprintTarget{{Role: RoleService, PID: 7}, {Role: RoleModel, PID: 8}})
	if len(out.Processes) != 2 {
		t.Fatalf("a model started after the cache was filled must show at once, got %d rows", len(out.Processes))
	}
}

func TestFootprintReusedPIDRestartsTheBaseline(t *testing.T) {
	clock := time.Unix(1000, 0)
	s := &fakeSampler{samples: map[int32]ProcSample{8: {CPUSeconds: 100, CreatedAt: time.Unix(10, 0)}}}
	r := newTestReader(s, nil, &clock)
	targets := []FootprintTarget{{Role: RoleModel, PID: 8}}
	r.Read(targets)
	clock = clock.Add(5 * time.Second)
	s.samples[8] = ProcSample{CPUSeconds: 1, CreatedAt: time.Unix(990, 0)}
	if got := r.Read(targets).Processes[0].CPUPercent; got != nil {
		t.Fatalf("a different process under the same pid must not produce a delta, got %v", *got)
	}
}

func TestFootprintSkipsAProcessThatIsGone(t *testing.T) {
	clock := time.Unix(1000, 0)
	s := &fakeSampler{samples: map[int32]ProcSample{7: {CreatedAt: time.Unix(1, 0)}}, err: map[int32]error{8: errors.New("no such process")}}
	r := newTestReader(s, nil, &clock)
	out := r.Read([]FootprintTarget{{Role: RoleService, PID: 7}, {Role: RoleModel, PID: 8}})
	if len(out.Processes) != 1 || out.Processes[0].PID != 7 {
		t.Fatalf("want only the live process, got %+v", out.Processes)
	}
}

func TestFootprintIgnoresInvalidPIDsAndRetainsTheRest(t *testing.T) {
	clock := time.Unix(1000, 0)
	s := &fakeSampler{samples: map[int32]ProcSample{7: {CreatedAt: time.Unix(1, 0)}}}
	r := newTestReader(s, nil, &clock)
	out := r.Read([]FootprintTarget{{Role: RoleService, PID: 7}, {Role: RoleModel, PID: 0}})
	if len(out.Processes) != 1 || s.calls != 1 {
		t.Fatalf("pid 0 (no model running) must be skipped without sampling, got %+v calls=%d", out.Processes, s.calls)
	}
	if len(s.retain) != 1 || s.retain[0] != 7 {
		t.Fatalf("sampler must keep only the live targets, got %v", s.retain)
	}
}

func TestFootprintReportsGPUMemoryOnlyWhenKnown(t *testing.T) {
	clock := time.Unix(1000, 0)
	s := &fakeSampler{samples: map[int32]ProcSample{7: {CreatedAt: time.Unix(1, 0)}, 8: {CreatedAt: time.Unix(1, 0)}}}
	g := fakeGPU{mem: map[int32]ProcGPUMem{8: {VRAMBytes: 2048 << 20, GTTBytes: 1024 << 20}}}
	r := newTestReader(s, g, &clock)
	out := r.Read([]FootprintTarget{{Role: RoleService, PID: 7}, {Role: RoleModel, PID: 8}})
	if out.Processes[0].GPUVRAMMB != nil {
		t.Fatalf("the service has no GPU reading; must be nil, got %v", *out.Processes[0].GPUVRAMMB)
	}
	if v := out.Processes[1].GPUVRAMMB; v == nil || *v != 2048 {
		t.Fatalf("VRAM = %v, want 2048", v)
	}
	if v := out.Processes[1].GPUGTTMB; v == nil || *v != 1024 {
		t.Fatalf("GTT = %v, want 1024", v)
	}
}

func TestParseNvidiaComputeApps(t *testing.T) {
	out := "1201, 5120\n3388, 256\n"
	if m, ok := parseNvidiaComputeApps(out, 3388); !ok || m.VRAMBytes != 256<<20 {
		t.Fatalf("got %+v ok=%v", m, ok)
	}
	if _, ok := parseNvidiaComputeApps(out, 7); ok {
		t.Fatal("a pid nvidia-smi does not list has no reading")
	}
}

// The real sampler on the test process: proves the gopsutil calls work on whatever OS runs the tests (Linux, macOS).
func TestGopsutilSamplerReadsThisProcess(t *testing.T) {
	s := newGopsutilSampler()
	// A fresh test binary can have used less CPU than one clock tick (10 ms on Linux), so burn some until a tick shows.
	var got ProcSample
	deadline := time.Now().Add(3 * time.Second)
	for got.CPUSeconds <= 0 && time.Now().Before(deadline) {
		for i := 0; i < 5_000_000; i++ {
			_ = i * i
		}
		var err error
		if got, err = s.Sample(int32(os.Getpid())); err != nil {
			t.Fatal(err)
		}
	}
	if got.RSSBytes == 0 || got.Threads == 0 || got.CreatedAt.IsZero() || got.CPUSeconds <= 0 {
		t.Fatalf("implausible reading of this process: %+v", got)
	}
	s.Retain(nil)
	if len(s.procs) != 0 {
		t.Fatal("Retain(nil) must drop every cached process")
	}
}

func TestReadGoRuntimeReportsLiveFigures(t *testing.T) {
	g := readGoRuntime()
	if g.Goroutines == 0 || g.HeapMB <= 0 || g.TotalMB < g.HeapMB {
		t.Fatalf("implausible Go runtime figures: %+v", g)
	}
}

func TestParseFdinfoSumsAmdgpuMemory(t *testing.T) {
	text := "pos:\t0\nflags:\t02\ndrm-driver:\tamdgpu\ndrm-client-id:\t42\ndrm-memory-vram:\t2048 KiB\ndrm-memory-gtt:\t1 MiB\n"
	id, mem, ok := parseFdinfo(text)
	if !ok || id != "amdgpu/42" || mem.VRAMBytes != 2048*1024 || mem.GTTBytes != 1<<20 {
		t.Fatalf("got id=%q mem=%+v ok=%v", id, mem, ok)
	}
}

func TestParseFdinfoIgnoresNonDRMFiles(t *testing.T) {
	if _, _, ok := parseFdinfo("pos:\t0\nflags:\t02\nmnt_id:\t12\n"); ok {
		t.Fatal("a file without a drm-client-id is not a GPU client")
	}
}

func TestFdinfoReaderCountsEachClientOnce(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "55", "fdinfo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	same := "drm-driver:\tamdgpu\ndrm-client-id:\t9\ndrm-memory-vram:\t1024 KiB\n"
	other := "drm-driver:\tamdgpu\ndrm-client-id:\t10\ndrm-memory-vram:\t1 MiB\n"
	for name, body := range map[string]string{"3": same, "4": same, "5": other, "6": "pos:\t0\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, ok := (&fdinfoGPUReader{procRoot: root}).ProcMem(55)
	if !ok || m.VRAMBytes != 2<<20 {
		t.Fatalf("want 2 MiB (client 9 once + client 10), got %+v ok=%v", m, ok)
	}
	if _, ok := (&fdinfoGPUReader{procRoot: root}).ProcMem(99); ok {
		t.Fatal("a pid with no fdinfo has no reading")
	}
}
