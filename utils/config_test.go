package utils_test

import (
	"reflect"
	"testing"

	"llm-proxy/models"
	"llm-proxy/utils"
)

func TestSaveAndLoadConfig_RoundTrip(t *testing.T) {
	cfg := &models.Config{
		Server: models.ServerConfig{
			Bind:              ":8080",
			ModelHost:         "http://localhost",
			IdleTimeoutSecs:   30,
			LlamaServerBinary: "llama-server",
			DefaultArgs:       []string{"--foo", "bar"},
		},
		Models: []models.ModelConfig{
			{Name: "alpha", Filename: "alpha.gguf", Args: []string{"--x"}, Port: 9000},
		},
		ModelDir: "/models",
		Metrics: models.MetricsConfig{
			GPU: models.GPUConfig{Provider: "none"},
		},
	}

	path := t.TempDir() + "/config.json"
	if err := utils.SaveConfig(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := utils.LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !reflect.DeepEqual(cfg, loaded) {
		t.Fatalf("config mismatch: %#v != %#v", cfg, loaded)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	cfg := &models.Config{Server: models.ServerConfig{Bind: ":0", IdleTimeoutSecs: 1, LlamaServerBinary: "old"}}

	t.Setenv("BIND", ":9090")
	t.Setenv("IDLE_TIMEOUT_SECS", "42")
	t.Setenv("LLAMA_BINARY", "new-binary")

	utils.LoadEnvOverrides(cfg)

	if cfg.Server.Bind != ":9090" {
		t.Fatalf("unexpected bind: %s", cfg.Server.Bind)
	}
	if cfg.Server.IdleTimeoutSecs != 42 {
		t.Fatalf("unexpected idle timeout: %d", cfg.Server.IdleTimeoutSecs)
	}
	if cfg.Server.LlamaServerBinary != "new-binary" {
		t.Fatalf("unexpected llama binary: %s", cfg.Server.LlamaServerBinary)
	}
}
