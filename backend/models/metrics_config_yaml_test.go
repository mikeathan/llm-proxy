package models

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMetricsConfig_YamlLegacyKeysStillLoad(t *testing.T) {
	// Pre-yaml-tag store format: field-name-derived keys.
	raw := `
metrics:
    gpu:
        provider: "auto"
        sysfspath: "/sys/class/drm/card0"
    gpusampleintervalsec: 7
    gpusmoothingalpha: 0.15
`
	var cfg AppConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal legacy yaml: %v", err)
	}
	if cfg.Metrics.GPUSampleIntervalSec != 7 {
		t.Fatalf("legacy gpusampleintervalsec: expected 7, got %d", cfg.Metrics.GPUSampleIntervalSec)
	}
	if cfg.Metrics.GPUSmoothingAlpha != 0.15 {
		t.Fatalf("legacy gpusmoothingalpha: expected 0.15, got %v", cfg.Metrics.GPUSmoothingAlpha)
	}
	if cfg.Metrics.GPU.SysfsPath != "/sys/class/drm/card0" {
		t.Fatalf("legacy sysfspath: expected path, got %q", cfg.Metrics.GPU.SysfsPath)
	}
}

func TestMetricsConfig_YamlCanonicalKeysLoad(t *testing.T) {
	raw := `
metrics:
    gpu:
        provider: "auto"
        sysfs_path: "/sys/class/drm/card0"
    gpu_sample_interval_seconds: 9
    gpu_smoothing_alpha: 0.2
`
	var cfg AppConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal canonical yaml: %v", err)
	}
	if cfg.Metrics.GPUSampleIntervalSec != 9 {
		t.Fatalf("canonical interval: expected 9, got %d", cfg.Metrics.GPUSampleIntervalSec)
	}
	if cfg.Metrics.GPUSmoothingAlpha != 0.2 {
		t.Fatalf("canonical alpha: expected 0.2, got %v", cfg.Metrics.GPUSmoothingAlpha)
	}
	if cfg.Metrics.GPU.SysfsPath != "/sys/class/drm/card0" {
		t.Fatalf("canonical sysfs_path: expected path, got %q", cfg.Metrics.GPU.SysfsPath)
	}
}

func TestMetricsConfig_YamlCanonicalKeyWinsOverLegacy(t *testing.T) {
	// A file carrying both keys (e.g. written by a pre-tag store, then edited)
	// must honour the canonical value.
	raw := `
metrics:
    gpusampleintervalsec: 7
    gpu_sample_interval_seconds: 11
    gpusmoothingalpha: 0.1
    gpu_smoothing_alpha: 0.3
`
	var cfg AppConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal mixed yaml: %v", err)
	}
	if cfg.Metrics.GPUSampleIntervalSec != 11 {
		t.Fatalf("expected canonical 11 to win, got %d", cfg.Metrics.GPUSampleIntervalSec)
	}
	if cfg.Metrics.GPUSmoothingAlpha != 0.3 {
		t.Fatalf("expected canonical 0.3 to win, got %v", cfg.Metrics.GPUSmoothingAlpha)
	}
}

func TestMetricsConfig_YamlMarshalWritesCanonicalKeys(t *testing.T) {
	cfg := AppConfig{
		Metrics: MetricsConfig{
			GPU: GPUConfig{
				Provider:  "auto",
				SysfsPath: "/sys/class/drm/card0",
			},
			GPUSampleIntervalSec: 7,
			GPUSmoothingAlpha:    0.15,
		},
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"gpu_sample_interval_seconds: 7",
		"gpu_smoothing_alpha: 0.15",
		"sysfs_path:",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("expected canonical key %q in marshalled yaml:\n%s", want, s)
		}
	}
	if strings.Contains(s, "gpusampleintervalsec") {
		t.Fatalf("marshal must not emit legacy keys:\n%s", s)
	}

	// Round-trip: canonical output loads back identically.
	var back AppConfig
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if back.Metrics != cfg.Metrics {
		t.Fatalf("round-trip mismatch: got %+v want %+v", back.Metrics, cfg.Metrics)
	}
}
