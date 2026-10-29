package app

import (
	"reflect"
	"testing"

	"llm-proxy/internal/platform/paths"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func registryWithConnectors(t *testing.T, conns map[string]models.ConnectorConfig) *storage.DataManager {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LLM_PROXY_HOME", dir)
	p := paths.Paths{ConfigDir: dir, DataDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	dm, err := storage.NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := dm.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if err := dm.Registry().Update(func(reg *models.RegistryData) error {
		reg.Communication.Connectors = conns
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	return dm
}

func TestEnsureWebhookSecrets(t *testing.T) {
	tg := func(enabled bool, url, token string) models.ConnectorConfig {
		return models.ConnectorConfig{
			Type: models.ConnectorTypeTelegram, Enabled: enabled, WebhookURL: url,
			Settings: map[string]string{"chat_id": "1", models.ConnectorSettingWebhookToken: token},
		}
	}
	dm := registryWithConnectors(t, map[string]models.ConnectorConfig{
		"registered-no-secret": tg(true, "https://h/api/v1/webhooks/a", ""),
		"has-secret":           tg(true, "https://h/api/v1/webhooks/b", "keep-me"),
		"never-registered":     tg(true, "", ""),
		"disabled":             tg(false, "https://h/api/v1/webhooks/d", ""),
	})

	if got, want := ensureWebhookSecrets(dm), []string{"registered-no-secret"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("given = %v, want %v", got, want)
	}
	conns := dm.Registry().Get().Communication.Connectors
	secret := conns["registered-no-secret"].Settings[models.ConnectorSettingWebhookToken]
	if len(secret) != 2*webhookSecretBytes {
		t.Errorf("new secret = %q, want %d hex characters", secret, 2*webhookSecretBytes)
	}
	if conns["registered-no-secret"].Settings["chat_id"] != "1" {
		t.Error("the connector's other settings must survive")
	}
	if conns["has-secret"].Settings[models.ConnectorSettingWebhookToken] != "keep-me" {
		t.Error("an existing secret must never be replaced")
	}
	for _, name := range []string{"never-registered", "disabled"} {
		if conns[name].Settings[models.ConnectorSettingWebhookToken] != "" {
			t.Errorf("%s was given a secret", name)
		}
	}
	if again := ensureWebhookSecrets(dm); len(again) != 0 {
		t.Errorf("second run changed %v, want nothing", again)
	}
}
