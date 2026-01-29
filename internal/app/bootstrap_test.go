package app

import (
	"testing"

	"llm-proxy/internal/mocks"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/testutils"
	"llm-proxy/models"
)

func TestBuildAppServices_UsesRuntimeProvider(t *testing.T) {
	testutils.SetRequiredEnv(t)

	logger := &mocks.MockLogger{}
	cfg := minimalConfig()

	container := bootstrap(cfg, logger)
	services := container.BuildAppServices()

	if _, ok := services.ClientProvider().(*proxy.RuntimeClientProvider); !ok {
		t.Fatalf("expected RuntimeClientProvider")
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
