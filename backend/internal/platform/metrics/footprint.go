package metrics

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// The service footprint: what the llm-proxy process and the model server it runs cost the machine — CPU, resident memory, threads,
// open files, uptime, GPU memory — plus the Go runtime's own figures. It is read only when the System stats panel is open (its own
// endpoint, not the 10 s /metrics poll on every page) and cached for footprintCacheInterval, so a panel left open costs a few
// procfs/libproc reads per poll. Nothing here walks the process table: the targets are the two pids the service already knows.

const (
	RoleService = "service"
	RoleModel   = "model"
)

// footprintCacheInterval bounds how often the process table is touched. CPU % is a delta between two reads, so reads closer
// together than this would only add noise.
const footprintCacheInterval = 2 * time.Second

const (
	bytesPerMB    = 1024 * 1024
	noOpenFiles   = -1
	fdinfoKeyVRAM = "drm-memory-vram"
	fdinfoKeyGTT  = "drm-memory-gtt"
)

// FootprintTarget names one process to measure.
type FootprintTarget struct {
	Role string
	PID  int
}

// ProcSample is one raw reading of a process; CPUSeconds is cumulative (user + system).
type ProcSample struct {
	CPUSeconds float64
	RSSBytes   uint64
	Threads    int32
	OpenFiles  int32 // noOpenFiles when the platform does not report it (macOS)
	CreatedAt  time.Time
}

// ProcSampler reads one process. Retain drops whatever it cached for pids that are no longer targets.
type ProcSampler interface {
	Sample(pid int32) (ProcSample, error)
	Retain(pids []int32)
}

// ProcGPUMem is the GPU memory one process holds. GTT is system memory the GPU maps (large on an APU).
type ProcGPUMem struct {
	VRAMBytes uint64
	GTTBytes  uint64
}

// ProcGPUReader reads a process's GPU memory; ok is false when the platform or driver does not report it.
type ProcGPUReader interface {
	ProcMem(pid int32) (ProcGPUMem, bool)
}

// ProcessFootprint is one row of the panel. Pointer fields are nil when not reported, never zero.
type ProcessFootprint struct {
	Role       string   `json:"role"`
	PID        int      `json:"pid"`
	CPUPercent *float64 `json:"cpu_percent"` // of one core, like top; nil until a second read gives a delta
	RSSMB      float64  `json:"rss_mb"`
	Threads    int      `json:"threads"`
	OpenFiles  *int     `json:"open_files,omitempty"`
	UptimeSec  float64  `json:"uptime_seconds"`
	GPUVRAMMB  *float64 `json:"gpu_vram_mb,omitempty"`
	GPUGTTMB   *float64 `json:"gpu_gtt_mb,omitempty"`
}

// GoRuntimeFootprint is the service's own Go runtime, read from runtime/metrics (no stop-the-world).
type GoRuntimeFootprint struct {
	HeapMB     float64 `json:"heap_mb"`
	TotalMB    float64 `json:"total_mb"`
	Goroutines uint64  `json:"goroutines"`
	GCCycles   uint64  `json:"gc_cycles"`
}

// Footprint is the whole answer of GET /admin/api/metrics/footprint.
type Footprint struct {
	Processes []ProcessFootprint `json:"processes"`
	Go        GoRuntimeFootprint `json:"go"`
	Timestamp time.Time          `json:"timestamp"`
}

type prevCPU struct {
	cpuSeconds float64
	at         time.Time
	createdAt  time.Time
}

// FootprintReader turns raw samples into a Footprint. One long-lived instance keeps the previous reading of each pid so CPU % is a
// delta; it is safe for concurrent callers.
type FootprintReader struct {
	sampler ProcSampler
	gpu     ProcGPUReader
	nowFn   func() time.Time

	mu       sync.Mutex
	prev     map[int32]prevCPU
	cached   Footprint
	cachedAt time.Time
	cacheKey string
}

// NewFootprintReader builds a reader; gpu may be nil (nothing reported).
func NewFootprintReader(sampler ProcSampler, gpu ProcGPUReader) *FootprintReader {
	return &FootprintReader{sampler: sampler, gpu: gpu, nowFn: time.Now, prev: map[int32]prevCPU{}}
}

// NewSystemFootprintReader reads the real processes of this machine (Linux and macOS).
func NewSystemFootprintReader() *FootprintReader {
	return NewFootprintReader(newGopsutilSampler(), newSystemGPUReader())
}

func targetsKey(targets []FootprintTarget) string {
	var b strings.Builder
	for _, t := range targets {
		b.WriteString(t.Role + ":" + strconv.Itoa(t.PID) + ",")
	}
	return b.String()
}

// Read returns the footprint of the targets. A target with pid <= 0 (no model running) or that has exited is left out.
func (r *FootprintReader) Read(targets []FootprintTarget) Footprint {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.nowFn()
	key := targetsKey(targets)
	if key == r.cacheKey && !r.cachedAt.IsZero() && now.Sub(r.cachedAt) < footprintCacheInterval {
		return r.cached
	}

	out := Footprint{Processes: []ProcessFootprint{}, Go: readGoRuntime(), Timestamp: now}
	live := make([]int32, 0, len(targets))
	for _, t := range targets {
		if t.PID <= 0 {
			continue
		}
		pid := int32(t.PID)
		sample, err := r.sampler.Sample(pid)
		if err != nil {
			continue
		}
		live = append(live, pid)
		out.Processes = append(out.Processes, r.row(t, sample, now))
	}
	r.sampler.Retain(live)
	for pid := range r.prev {
		if !containsPID(live, pid) {
			delete(r.prev, pid)
		}
	}
	r.cached, r.cachedAt, r.cacheKey = out, now, key
	return out
}

func containsPID(pids []int32, pid int32) bool {
	for _, p := range pids {
		if p == pid {
			return true
		}
	}
	return false
}

func (r *FootprintReader) row(t FootprintTarget, s ProcSample, now time.Time) ProcessFootprint {
	row := ProcessFootprint{
		Role:       t.Role,
		PID:        t.PID,
		CPUPercent: r.cpuPercent(int32(t.PID), s, now),
		RSSMB:      float64(s.RSSBytes) / bytesPerMB,
		Threads:    int(s.Threads),
	}
	if s.OpenFiles != noOpenFiles {
		n := int(s.OpenFiles)
		row.OpenFiles = &n
	}
	if !s.CreatedAt.IsZero() {
		row.UptimeSec = math.Max(0, now.Sub(s.CreatedAt).Seconds())
	}
	// Only the model server holds GPU memory; asking for the service would cost an nvidia-smi run for nothing.
	if r.gpu != nil && t.Role == RoleModel {
		if m, ok := r.gpu.ProcMem(int32(t.PID)); ok {
			vram, gtt := float64(m.VRAMBytes)/bytesPerMB, float64(m.GTTBytes)/bytesPerMB
			row.GPUVRAMMB = &vram
			if m.GTTBytes > 0 {
				row.GPUGTTMB = &gtt
			}
		}
	}
	return row
}

// cpuPercent is the CPU time used since the previous read divided by the wall time between them. A pid first seen, or reused by a
// different process (its start time changed), has no previous read and reports nil.
func (r *FootprintReader) cpuPercent(pid int32, s ProcSample, now time.Time) *float64 {
	prev, seen := r.prev[pid]
	r.prev[pid] = prevCPU{cpuSeconds: s.CPUSeconds, at: now, createdAt: s.CreatedAt}
	if !seen || !prev.createdAt.Equal(s.CreatedAt) {
		return nil
	}
	dt := now.Sub(prev.at).Seconds()
	if dt <= 0 {
		return nil
	}
	pct := math.Max(0, (s.CPUSeconds-prev.cpuSeconds)/dt*100)
	return &pct
}

var goRuntimeSamples = []string{
	"/memory/classes/heap/objects:bytes",
	"/memory/classes/total:bytes",
	"/sched/goroutines:goroutines",
	"/gc/cycles/total:gc-cycles",
}

func readGoRuntime() GoRuntimeFootprint {
	samples := make([]metrics.Sample, len(goRuntimeSamples))
	for i, name := range goRuntimeSamples {
		samples[i].Name = name
	}
	metrics.Read(samples)
	u64 := func(i int) uint64 {
		if samples[i].Value.Kind() != metrics.KindUint64 {
			return 0
		}
		return samples[i].Value.Uint64()
	}
	return GoRuntimeFootprint{
		HeapMB:     float64(u64(0)) / bytesPerMB,
		TotalMB:    float64(u64(1)) / bytesPerMB,
		Goroutines: u64(2),
		GCCycles:   u64(3),
	}
}

// gopsutilSampler reads processes through gopsutil: procfs on Linux, libproc on macOS when built with cgo (the default there; a
// build without cgo makes gopsutil run `ps` for each figure). It uses cumulative CPU times, not Process.Percent, so no hidden
// per-process state is shared between callers; the one *Process per pid is only a cache of its start time.
type gopsutilSampler struct {
	mu    sync.Mutex
	procs map[int32]*process.Process
}

func newGopsutilSampler() *gopsutilSampler {
	return &gopsutilSampler{procs: map[int32]*process.Process{}}
}

func (g *gopsutilSampler) proc(pid int32) (*process.Process, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if p, ok := g.procs[pid]; ok {
		return p, nil
	}
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}
	g.procs[pid] = p
	return p, nil
}

func (g *gopsutilSampler) Sample(pid int32) (ProcSample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p, err := g.proc(pid)
	if err != nil {
		return ProcSample{}, err
	}
	times, err := p.TimesWithContext(ctx)
	if err != nil {
		g.forget(pid)
		return ProcSample{}, err
	}
	mem, err := p.MemoryInfoWithContext(ctx)
	if err != nil {
		g.forget(pid)
		return ProcSample{}, err
	}
	s := ProcSample{CPUSeconds: times.User + times.System, RSSBytes: mem.RSS, OpenFiles: noOpenFiles}
	if n, err := p.NumThreadsWithContext(ctx); err == nil {
		s.Threads = n
	}
	if n, err := p.NumFDsWithContext(ctx); err == nil {
		s.OpenFiles = n
	}
	if ms, err := p.CreateTimeWithContext(ctx); err == nil {
		s.CreatedAt = time.UnixMilli(ms)
	}
	return s, nil
}

func (g *gopsutilSampler) forget(pid int32) {
	g.mu.Lock()
	delete(g.procs, pid)
	g.mu.Unlock()
}

func (g *gopsutilSampler) Retain(pids []int32) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for pid := range g.procs {
		if !containsPID(pids, pid) {
			delete(g.procs, pid)
		}
	}
}

// newSystemGPUReader reads per-process GPU memory where the OS exposes it: DRM fdinfo on Linux (amdgpu, Intel), then nvidia-smi
// when it is installed. macOS reports nothing (Metal memory is unified and not attributed per process).
func newSystemGPUReader() ProcGPUReader {
	if runtime.GOOS != "linux" {
		return nil
	}
	readers := []ProcGPUReader{&fdinfoGPUReader{procRoot: "/proc"}}
	if commandExists("nvidia-smi") {
		readers = append(readers, &nvidiaProcReader{binary: "nvidia-smi", run: defaultCommandRunner})
	}
	return chainGPUReader(readers)
}

type chainGPUReader []ProcGPUReader

func (c chainGPUReader) ProcMem(pid int32) (ProcGPUMem, bool) {
	for _, r := range c {
		if m, ok := r.ProcMem(pid); ok {
			return m, true
		}
	}
	return ProcGPUMem{}, false
}

// fdinfoGPUReader sums the DRM memory a process holds, from /proc/<pid>/fdinfo/*. Each open GPU client appears once per file
// descriptor that shares it, so clients are counted by their drm-client-id. A pid that cannot be read (another user) reports nothing.
type fdinfoGPUReader struct{ procRoot string }

func (f *fdinfoGPUReader) ProcMem(pid int32) (ProcGPUMem, bool) {
	dir := filepath.Join(f.procRoot, strconv.Itoa(int(pid)), "fdinfo")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ProcGPUMem{}, false
	}
	seen := map[string]bool{}
	var total ProcGPUMem
	found := false
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		id, mem, ok := parseFdinfo(string(b))
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		found = true
		total.VRAMBytes += mem.VRAMBytes
		total.GTTBytes += mem.GTTBytes
	}
	return total, found
}

// parseFdinfo reads one fdinfo file; ok is false when it is not a DRM client. The id joins driver and client id (ids are per driver).
func parseFdinfo(text string) (id string, mem ProcGPUMem, ok bool) {
	driver, client := "", ""
	haveVRAM, haveGTT := false, false
	for _, line := range strings.Split(text, "\n") {
		key, val, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "drm-driver":
			driver = val
		case "drm-client-id":
			client = val
		case fdinfoKeyVRAM:
			mem.VRAMBytes, haveVRAM = parseSizeWithUnit(val), true
		case fdinfoKeyGTT:
			mem.GTTBytes, haveGTT = parseSizeWithUnit(val), true
		}
	}
	// A client that reports no memory line (Intel i915/xe name theirs drm-total-*, not read here) is not a reading: claiming one would
	// show 0 MB and keep the nvidia-smi fallback from running.
	if client == "" || (!haveVRAM && !haveGTT) {
		return "", ProcGPUMem{}, false
	}
	return driver + "/" + client, mem, true
}

// parseSizeWithUnit reads "2048 KiB" style values (a bare number is bytes); 0 when it does not parse.
func parseSizeWithUnit(v string) uint64 {
	num, unit, _ := strings.Cut(v, " ")
	n, err := strconv.ParseUint(strings.TrimSpace(num), 10, 64)
	if err != nil {
		return 0
	}
	switch strings.TrimSpace(unit) {
	case "KiB":
		return n << 10
	case "MiB":
		return n << 20
	case "GiB":
		return n << 30
	}
	return n
}

// nvidiaProcReader asks nvidia-smi which compute processes hold memory. It runs only when fdinfo reported nothing, in the panel's
// poll, with a short timeout.
type nvidiaProcReader struct {
	binary string
	run    commandRunner
}

func (n *nvidiaProcReader) ProcMem(pid int32) (ProcGPUMem, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := n.run(ctx, n.binary, "--query-compute-apps=pid,used_memory", "--format=csv,noheader,nounits")
	if err != nil {
		return ProcGPUMem{}, false
	}
	return parseNvidiaComputeApps(string(out), pid)
}

// parseNvidiaComputeApps finds pid in "pid, used_memory(MiB)" lines.
func parseNvidiaComputeApps(text string, pid int32) (ProcGPUMem, bool) {
	for _, line := range strings.Split(text, "\n") {
		a, b, ok := strings.Cut(line, ",")
		if !ok || strings.TrimSpace(a) != strconv.Itoa(int(pid)) {
			continue
		}
		mib, err := strconv.ParseUint(strings.TrimSpace(b), 10, 64)
		if err != nil {
			return ProcGPUMem{}, false
		}
		return ProcGPUMem{VRAMBytes: mib << 20}, true
	}
	return ProcGPUMem{}, false
}
