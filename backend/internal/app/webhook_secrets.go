package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// webhookSecretBytes is the size of a generated webhook secret before hex encoding: 256 bits, and 64 characters, inside
// the 1-256 characters of letters, digits, underscore and hyphen that Telegram accepts.
const webhookSecretBytes = 32

// newWebhookSecret is a random webhook secret.
func newWebhookSecret() (string, error) {
	b := make([]byte, webhookSecretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// lacksRegisteredSecret is an enabled connector with a registered webhook and no webhook secret.
func lacksRegisteredSecret(cfg models.ConnectorConfig) bool {
	return cfg.Enabled && cfg.WebhookURL != "" && cfg.Settings[models.ConnectorSettingWebhookToken] == ""
}

// ensureWebhookSecrets gives every enabled connector that has a registered webhook and no webhook secret a new one, and
// returns the names it changed. The inbound endpoint refuses a connector with no secret, so a connector set up before
// that rule would stop receiving; running this before the connectors are built lets their startup re-registration tell
// the platform the new secret in the same boot. A connector that fails stays as it was and is logged.
func ensureWebhookSecrets(dataMgr *storage.DataManager) []string {
	// A save notifies the registry's subscribers, so a boot with nothing to change must not write.
	if !slices.ContainsFunc(slices.Collect(maps.Values(dataMgr.Registry().Get().Communication.Connectors)), lacksRegisteredSecret) {
		return nil
	}
	var given []string
	err := dataMgr.Registry().Update(func(reg *models.RegistryData) error {
		for name, cfg := range reg.Communication.Connectors {
			if !lacksRegisteredSecret(cfg) {
				continue
			}
			secret, err := newWebhookSecret()
			if err != nil {
				logging.Warn("webhook secret not created", "connector", name, "error", err)
				continue
			}
			settings := make(map[string]string, len(cfg.Settings)+1)
			for k, v := range cfg.Settings {
				settings[k] = v
			}
			settings[models.ConnectorSettingWebhookToken] = secret
			cfg.Settings = settings
			reg.Communication.Connectors[name] = cfg
			given = append(given, name)
		}
		return nil
	})
	if err != nil {
		logging.Warn("webhook secrets not saved", "error", err)
		return nil
	}
	slices.Sort(given)
	for _, name := range given {
		logging.Info("webhook secret created for a connector that had none; its webhook is registered again with it", "connector", name)
	}
	return given
}
