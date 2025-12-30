package system_metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/models"
)

func TestParseAmdGpuTopJSON_DevicesArray(t *testing.T) {
	raw := `{"devices":[{"name":"card0","GFX Activity":45,"VRAM":{"Usage VRAM [MiB]":100,"Total VRAM [MiB]":200},"Temperature (C)":70}]}`

	snap, err := parseAmdGpuTopJSON([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Name != "card0" {
		t.Fatalf("unexpected name: %s", snap.Name)
	}
	if snap.UtilizationPct != 45 {
		t.Fatalf("unexpected utilization: %v", snap.UtilizationPct)
	}
	if snap.MemoryUsedMB != 100 || snap.MemoryTotalMB != 200 {
		t.Fatalf("unexpected memory: used=%v total=%v", snap.MemoryUsedMB, snap.MemoryTotalMB)
	}
	if snap.MemoryUtilizationPct != 50 {
		t.Fatalf("unexpected memory percent: %v", snap.MemoryUtilizationPct)
	}
	if snap.TemperatureC != 70 {
		t.Fatalf("unexpected temperature: %v", snap.TemperatureC)
	}
}

func TestParseAmdGpuTopJSON_FlatObject(t *testing.T) {
	raw := `{"asic":"navy","activity":"12 %"}`

	snap, err := parseAmdGpuTopJSON([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Name != "navy" {
		t.Fatalf("unexpected name: %s", snap.Name)
	}
	if snap.UtilizationPct != 12 {
		t.Fatalf("unexpected utilization: %v", snap.UtilizationPct)
	}
}

func TestSysfsProviderSample(t *testing.T) {
	base := t.TempDir()

	writeFile(t, filepath.Join(base, "gpu_busy_percent"), "25")
	writeFile(t, filepath.Join(base, "mem_info_vram_used"), "104857600")
	writeFile(t, filepath.Join(base, "mem_info_vram_total"), "209715200")
	writeFile(t, filepath.Join(base, "hwmon", "hwmon0", "temp1_input"), "42000")

	provider := newSysfsProvider(base)
	snap, err := provider.Sample()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.UtilizationPct != 25 {
		t.Fatalf("unexpected utilization: %v", snap.UtilizationPct)
	}
	if snap.MemoryUsedMB != 100 || snap.MemoryTotalMB != 200 {
		t.Fatalf("unexpected memory: used=%v total=%v", snap.MemoryUsedMB, snap.MemoryTotalMB)
	}
	if snap.MemoryUtilizationPct != 50 {
		t.Fatalf("unexpected memory percent: %v", snap.MemoryUtilizationPct)
	}
	if snap.TemperatureC != 42 {
		t.Fatalf("unexpected temperature: %v", snap.TemperatureC)
	}
}

func TestBuildGPUProvider_SysfsPath(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "gpu_busy_percent"), "1")

	cfg := &models.Config{Metrics: models.MetricsConfig{GPU: models.GPUConfig{Provider: "sysfs", SysfsPath: base}}}
	provider, name, err := buildGPUProvider(cfg)
	if provider == nil || name != "sysfs" || err != "" {
		t.Fatalf("unexpected result: provider=%v name=%s err=%s", provider, name, err)
	}

	cfg.Metrics.GPU.SysfsPath = filepath.Join(base, "missing")
	provider, name, err = buildGPUProvider(cfg)
	if provider != nil {
		t.Fatalf("expected nil provider for missing sysfs path")
	}
	if name == "" || !strings.Contains(err, "not readable") {
		t.Fatalf("expected readable error for missing path, got name=%s err=%s", name, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
