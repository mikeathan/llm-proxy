package system_metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"llm-proxy/models"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var numberRe = regexp.MustCompile(`-?\d+(\.\d+)?`)

func buildGPUProvider(cfg *models.Config) (GPUProvider, string, string) {
	var gpuCfg models.GPUConfig
	if cfg != nil {
		gpuCfg = cfg.Metrics.GPU
	}

	provider := strings.ToLower(strings.TrimSpace(gpuCfg.Provider))
	binary := strings.TrimSpace(gpuCfg.Binary)
	index := gpuCfg.Index
	sysfsPath := strings.TrimSpace(gpuCfg.SysfsPath)

	if provider == "none" || provider == "off" || provider == "disabled" {
		return nil, "disabled", ""
	}

	if provider == "" || provider == "auto" {
		if p, name := tryNvidia(binary, index); p != nil {
			return p, name, ""
		}
		if p, name := tryRocm(binary, index); p != nil {
			return p, name, ""
		}
		if p, name := tryAmdGpuTop(binary, index); p != nil {
			return p, name, ""
		}
		if p, name := trySysfs(sysfsPath, index); p != nil {
			return p, name, ""
		}
		return nil, "auto", "no GPU metrics source found (tried nvidia-smi, rocm-smi, amd-smi, amdgpu_top, sysfs)"
	}

	switch provider {
	case "nvidia", "nvidia-smi":
		bin := binary
		if bin == "" {
			bin = "nvidia-smi"
		}
		if !commandExists(bin) {
			return nil, bin, fmt.Sprintf("%s not found on PATH", bin)
		}
		return newNvidiaSMIProvider(bin, index, nil), bin, ""
	case "rocm", "amd", "rocm-smi", "amd-smi":
		bin := binary
		if bin == "" {
			if commandExists("rocm-smi") {
				bin = "rocm-smi"
			} else {
				bin = "amd-smi"
			}
		}
		if !commandExists(bin) {
			return nil, bin, fmt.Sprintf("%s not found on PATH", bin)
		}
		return newRocmSMIProvider(bin, index, nil), bin, ""
	case "amdgpu_top", "amdgpu-top":
		bin := binary
		if bin == "" {
			bin = "amdgpu_top"
		}
		if !commandExists(bin) {
			return nil, bin, fmt.Sprintf("%s not found on PATH", bin)
		}
		return newAmdGpuTopProvider(bin, index, nil), bin, ""
	case "sysfs":
		path := sysfsPath
		if path == "" {
			path = defaultSysfsPath(index)
		}
		if ok := sysfsAvailable(path); !ok {
			return nil, path, fmt.Sprintf("sysfs path %s not readable", path)
		}
		return newSysfsProvider(path), "sysfs", ""
	default:
		if binary == "" {
			binary = provider
		}
		if commandExists(binary) {
			return nil, provider, fmt.Sprintf("unsupported gpu provider %q (binary %s found but no parser)", provider, binary)
		}
		return nil, provider, fmt.Sprintf("%s not found on PATH", binary)
	}
}

func tryNvidia(binary string, index int) (GPUProvider, string) {
	bin := binary
	if bin == "" {
		bin = "nvidia-smi"
	}
	if !commandExists(bin) {
		return nil, ""
	}
	return newNvidiaSMIProvider(bin, index, nil), bin
}

func tryRocm(binary string, index int) (GPUProvider, string) {
	candidates := []string{}
	if binary != "" {
		candidates = append(candidates, binary)
	} else {
		candidates = append(candidates, "rocm-smi", "amd-smi")
	}

	for _, bin := range candidates {
		if commandExists(bin) {
			return newRocmSMIProvider(bin, index, nil), bin
		}
	}
	return nil, ""
}

func tryAmdGpuTop(binary string, index int) (GPUProvider, string) {
	bin := binary
	if bin == "" {
		bin = "amdgpu_top"
	}
	if commandExists(bin) {
		return newAmdGpuTopProvider(bin, index, nil), bin
	}
	return nil, ""
}

func trySysfs(path string, index int) (GPUProvider, string) {
	if path == "" {
		path = defaultSysfsPath(index)
	}
	if sysfsAvailable(path) {
		return newSysfsProvider(path), "sysfs"
	}
	return nil, ""
}

func defaultCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func commandExists(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

func newNvidiaSMIProvider(binary string, index int, runner commandRunner) *nvidiaSMIProvider {
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &nvidiaSMIProvider{
		binary: binary,
		index:  index,
		run:    runner,
	}
}

func (p *nvidiaSMIProvider) Name() string {
	return p.binary
}

func (p *nvidiaSMIProvider) Sample() (*GPUMetrics, error) {
	args := []string{"--query-gpu=utilization.gpu,memory.used,memory.total,temperature.gpu", "--format=csv,noheader,nounits"}
	if p.index > 0 {
		args = append(args, "-i", strconv.Itoa(p.index))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := p.run(ctx, p.binary, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w (%s)", p.binary, err, strings.TrimSpace(string(out)))
	}

	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	parts := strings.Split(line, ",")
	if len(parts) < 3 {
		return nil, fmt.Errorf("unexpected %s output", p.binary)
	}

	parse := func(s string) float64 {
		v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return v
	}

	util := parse(parts[0])
	memUsed := parse(parts[1])
	memTotal := parse(parts[2])
	temp := 0.0
	if len(parts) > 3 {
		temp = parse(parts[3])
	}

	memPct := 0.0
	if memTotal > 0 {
		memPct = (memUsed / memTotal) * 100
	}

	return &GPUMetrics{
		Vendor:               "nvidia",
		UtilizationPct:       util,
		MemoryUsedMB:         memUsed,
		MemoryTotalMB:        memTotal,
		MemoryUtilizationPct: memPct,
		TemperatureC:         temp,
	}, nil
}

func newRocmSMIProvider(binary string, index int, runner commandRunner) *rocmSMIProvider {
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &rocmSMIProvider{
		binary: binary,
		index:  index,
		run:    runner,
	}
}

func (p *rocmSMIProvider) Name() string {
	return p.binary
}

func (p *rocmSMIProvider) Sample() (*GPUMetrics, error) {
	args := []string{"--json", "--showuse", "--showmeminfo", "vram", "--showtemp"}
	if p.index > 0 {
		args = append(args, "--device", strconv.Itoa(p.index))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := p.run(ctx, p.binary, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w (%s)", p.binary, err, strings.TrimSpace(string(out)))
	}

	snap, parseErr := parseRocmSMIOutput(out)
	if parseErr != nil {
		return nil, fmt.Errorf("%s parse: %w", p.binary, parseErr)
	}
	return snap, nil
}

func parseRocmSMIOutput(raw []byte) (*GPUMetrics, error) {
	var payload map[string]map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse rocm-smi output: %w", err)
	}
	if len(payload) == 0 {
		return nil, errors.New("no GPU entries in rocm-smi output")
	}

	for key, data := range payload {
		util := extractPercent(data, "gpu use")
		if util == 0 {
			util = extractPercent(data, "gpu activity")
		}

		memUsed, memTotal := extractMemoryMB(data)
		memPct := 0.0
		if memTotal > 0 {
			memPct = (memUsed / memTotal) * 100
		}

		temp := extractTemperature(data)

		return &GPUMetrics{
			Vendor:               "amd",
			Name:                 key,
			UtilizationPct:       util,
			MemoryUsedMB:         memUsed,
			MemoryTotalMB:        memTotal,
			MemoryUtilizationPct: memPct,
			TemperatureC:         temp,
		}, nil
	}

	return nil, errors.New("unable to find GPU metrics in rocm-smi output")
}

func extractPercent(data map[string]interface{}, keyContains string) float64 {
	for k, v := range data {
		lk := strings.ToLower(k)
		if strings.Contains(lk, keyContains) {
			if pct := parseNumber(v); pct > 0 {
				return pct
			}
		}
	}
	return 0
}

func extractMemoryMB(data map[string]interface{}) (float64, float64) {
	vramUsed := firstNumberForKeys(data, []string{"vram total used", "vram usage", "vram used"})
	gttUsed := firstNumberForKeys(data, []string{"gtt total used", "gtt usage", "gtt used"})
	visUsed := firstNumberForKeys(data, []string{"vis_vram total used", "vis_vram usage", "vis_vram used"})

	vramTotal := firstNumberForKeys(data, []string{"vram total memory", "vram total"})
	gttTotal := firstNumberForKeys(data, []string{"gtt total memory", "gtt total"})
	visTotal := firstNumberForKeys(data, []string{"vis_vram total memory", "vis_vram total"})

	totalUsed := vramUsed + gttUsed + visUsed
	totalTotal := vramTotal + gttTotal + visTotal

	if totalUsed == 0 && totalTotal == 0 {
		totalUsed = firstNumberForKeys(data, []string{"used (b)", "usage (b)", "memory used"})
		totalTotal = firstNumberForKeys(data, []string{"total (b)", "memory total"})
	}

	return bytesToMB(totalUsed), bytesToMB(totalTotal)
}

func extractTemperature(data map[string]interface{}) float64 {
	for k, v := range data {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "(c)") || strings.Contains(lk, "temp") {
			if t := parseNumber(v); t > 0 {
				return t
			}
		}
	}
	return 0
}

func firstNumberForKeys(data map[string]interface{}, keys []string) float64 {
	for _, key := range keys {
		for k, v := range data {
			if strings.Contains(strings.ToLower(k), key) {
				if n := parseNumber(v); n > 0 {
					return n
				}
			}
		}
	}
	return 0
}

func parseNumber(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		if m := numberRe.FindString(t); m != "" {
			f, _ := strconv.ParseFloat(m, 64)
			return f
		}
	}
	return 0
}

func bytesToMB(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return v / (1024.0 * 1024.0)
}

func newSysfsProvider(basePath string) *sysfsProvider {
	return &sysfsProvider{basePath: basePath}
}

func (p *sysfsProvider) Name() string {
	return "sysfs"
}

func (p *sysfsProvider) Sample() (*GPUMetrics, error) {
	util, err := readSysfsFloat(p.basePath, "gpu_busy_percent")
	if err != nil {
		return nil, err
	}
	memUsedBytes, err := readSysfsFloat(p.basePath, "mem_info_vram_used")
	if err != nil {
		return nil, err
	}
	memTotalBytes, err := readSysfsFloat(p.basePath, "mem_info_vram_total")
	if err != nil {
		return nil, err
	}

	gttUsedBytes, _ := readSysfsFloat(p.basePath, "mem_info_gtt_used")
	gttTotalBytes, _ := readSysfsFloat(p.basePath, "mem_info_gtt_total")
	visUsedBytes, _ := readSysfsFloat(p.basePath, "mem_info_vis_vram_used")
	visTotalBytes, _ := readSysfsFloat(p.basePath, "mem_info_vis_vram_total")

	memUsedMB := bytesToMB(memUsedBytes + gttUsedBytes + visUsedBytes)
	memTotalMB := bytesToMB(memTotalBytes + gttTotalBytes + visTotalBytes)
	memPct := 0.0
	if memTotalMB > 0 {
		memPct = (memUsedMB / memTotalMB) * 100
	}

	tempC := 0.0
	if t, err := readSysfsTempC(p.basePath); err == nil {
		tempC = t
	}

	return &GPUMetrics{
		Vendor:               "amd",
		Name:                 filepath.Base(p.basePath),
		UtilizationPct:       util,
		MemoryUsedMB:         memUsedMB,
		MemoryTotalMB:        memTotalMB,
		MemoryUtilizationPct: memPct,
		TemperatureC:         tempC,
	}, nil
}

func readSysfsFloat(basePath, file string) (float64, error) {
	b, err := os.ReadFile(filepath.Join(basePath, file))
	if err != nil {
		return 0, fmt.Errorf("sysfs: %w", err)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	if err != nil {
		return 0, fmt.Errorf("sysfs parse: %w", err)
	}
	return v, nil
}

// readSysfsTempC tries common hwmon temp paths under the device for AMD GPUs.
func readSysfsTempC(basePath string) (float64, error) {
	matches, err := filepath.Glob(filepath.Join(basePath, "hwmon", "hwmon*", "temp1_input"))
	if err != nil || len(matches) == 0 {
		return 0, fmt.Errorf("sysfs temp: no hwmon temp1_input found")
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		return 0, fmt.Errorf("sysfs temp: %w", err)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	if err != nil {
		return 0, fmt.Errorf("sysfs temp parse: %w", err)
	}
	// Most drivers expose millidegrees; normalize to Celsius if so.
	if v > 200 {
		v = v / 1000.0
	}
	return v, nil
}

func defaultSysfsPath(index int) string {
	card := fmt.Sprintf("card%d", index)
	return filepath.Join("/sys/class/drm", card, "device")
}

func sysfsAvailable(basePath string) bool {
	_, err := os.Stat(filepath.Join(basePath, "gpu_busy_percent"))
	return err == nil
}

func newAmdGpuTopProvider(binary string, index int, runner commandRunner) *amdGpuTopProvider {
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &amdGpuTopProvider{
		binary: binary,
		index:  index,
		run:    runner,
	}
}

func (p *amdGpuTopProvider) Name() string {
	return p.binary
}

func (p *amdGpuTopProvider) Sample() (*GPUMetrics, error) {
	args := []string{"--json"}
	if p.index > 0 {
		args = append(args, "--device", strconv.Itoa(p.index))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := p.run(ctx, p.binary, args...)
	if err != nil {
		// Older amdgpu_top uses -J; retry once for compatibility.
		argsCompat := []string{"-J"}
		if p.index > 0 {
			argsCompat = append(argsCompat, "--device", strconv.Itoa(p.index))
		}
		outCompat, errCompat := p.run(ctx, p.binary, argsCompat...)
		if errCompat != nil {
			return nil, fmt.Errorf("%s: %w (%s)", p.binary, err, strings.TrimSpace(string(out)))
		}
		out = outCompat
	}

	snap, parseErr := parseAmdGpuTopJSON(out)
	if parseErr != nil {
		return nil, fmt.Errorf("%s parse: %w", p.binary, parseErr)
	}
	return snap, nil
}

func parseAmdGpuTopJSON(raw []byte) (*GPUMetrics, error) {
	// amdgpu_top JSON can be an object with a "devices" array or a flat object.
	var any interface{}
	if err := json.Unmarshal(raw, &any); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	entries := []map[string]interface{}{}
	switch t := any.(type) {
	case map[string]interface{}:
		// devices list
		if devs, ok := t["devices"].([]interface{}); ok {
			for _, d := range devs {
				if m, ok := d.(map[string]interface{}); ok {
					entries = append(entries, m)
				}
			}
		}
		// single device object
		if len(entries) == 0 {
			entries = append(entries, t)
		}
	case []interface{}:
		for _, d := range t {
			if m, ok := d.(map[string]interface{}); ok {
				entries = append(entries, m)
			}
		}
	}

	if len(entries) == 0 {
		return nil, errors.New("no GPU entries in amdgpu_top output")
	}

	device := entries[0]
	util := findNestedPercent(device, []string{"gfx activity", "gpu activity", "gpu use", "gfx %"})
	if util == 0 {
		util = extractPercent(device, "activity")
	}

	memUsed, memTotal := findNestedMemory(device)
	memPct := 0.0
	if memTotal > 0 {
		memPct = (memUsed / memTotal) * 100
	}

	temp := findNestedTemperature(device)

	name := ""
	if v, ok := device["name"].(string); ok {
		name = v
	}
	if asic, ok := device["asic"].(string); ok && name == "" {
		name = asic
	}

	return &GPUMetrics{
		Vendor:               "amd",
		Name:                 name,
		UtilizationPct:       util,
		MemoryUsedMB:         memUsed,
		MemoryTotalMB:        memTotal,
		MemoryUtilizationPct: memPct,
		TemperatureC:         temp,
	}, nil
}

func findNestedPercent(data map[string]interface{}, keyHints []string) float64 {
	for _, hint := range keyHints {
		if v := firstNumberForKeys(data, []string{hint}); v > 0 {
			return v
		}
	}
	// look inside nested maps
	for _, v := range data {
		if m, ok := v.(map[string]interface{}); ok {
			if pct := findNestedPercent(m, keyHints); pct > 0 {
				return pct
			}
		}
	}
	return 0
}

func findNestedMemory(data map[string]interface{}) (float64, float64) {
	used := firstNumberForKeys(data, []string{"usage vram", "used vram", "usage vram [mib]", "usage vram [mb]", "vram usage", "vram used", "used vram [mib]", "usage gtt", "used gtt", "usage gtt [mib]", "usage gtt [mb]", "gtt usage", "gtt used"})
	total := firstNumberForKeys(data, []string{"total vram", "total vram [mib]", "total vram [mb]", "vram total", "total gtt", "total gtt [mib]", "total gtt [mb]", "gtt total"})

	for _, v := range data {
		if m, ok := v.(map[string]interface{}); ok {
			u, t := findNestedMemory(m)
			used += u
			total += t
		}
	}

	// amdgpu_top reports in MiB already
	return used, total
}

func findNestedTemperature(data map[string]interface{}) float64 {
	for k, v := range data {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "temp") || strings.Contains(lk, "(c)") || strings.Contains(lk, "edge") {
			if t := parseNumber(v); t > 0 {
				return t
			}
		}
		if m, ok := v.(map[string]interface{}); ok {
			if t := findNestedTemperature(m); t > 0 {
				return t
			}
		}
	}
	return 0
}
