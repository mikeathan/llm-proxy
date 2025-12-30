package app

import (
	"testing"

	"llm-proxy/internal/proxy"
	"llm-proxy/models"
)

func TestBuildAppServices_UsesDevBaseURL(t *testing.T) {
	t.Setenv("DEVICE_CONTEXT_BASE_URL", "http://mock-device-context")
	t.Setenv("LLM_PROXY_DEV_BASE_URL", "http://mock-llm")

	cfg := minimalConfig()
	container := bootstrap(cfg)
	services := container.BuildAppServices()

	if _, ok := services.ClientProvider().(*proxy.StaticClientProvider); !ok {
		t.Fatalf("expected StaticClientProvider when LLM_PROXY_DEV_BASE_URL is set")
	}
}

func TestBuildAppServices_UsesRuntimeProviderByDefault(t *testing.T) {
	t.Setenv("DEVICE_CONTEXT_BASE_URL", "http://mock-device-context")

	cfg := minimalConfig()
	container := bootstrap(cfg)
	services := container.BuildAppServices()

	if _, ok := services.ClientProvider().(*proxy.RuntimeClientProvider); !ok {
		t.Fatalf("expected RuntimeClientProvider when LLM_PROXY_DEV_BASE_URL is not set")
	}
}

func minimalConfig() *models.Config {
	return &models.Config{
		Server: models.ServerConfig{
			Bind:            ":0",
			ModelHost:       "http://localhost",
			IdleTimeoutSecs: 10,
		},
		Models:   []models.ModelConfig{},
		ModelDir: "",
		Metrics: models.MetricsConfig{
			GPU: models.GPUConfig{
				Provider: "none",
			},
		},
	}
}
