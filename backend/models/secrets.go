package models

type Secrets struct {
	ProviderKeys map[string][]APIKeyItem `json:"provider_keys,omitempty"`
	TavilyKey    string                  `json:"tavily_key,omitempty"`
	TelegramToken string                 `json:"telegram_token,omitempty"`
}
