package handlers

import (
	"testing"

	"llm-proxy/models"
)

func inboundConnector(secret string) models.ConnectorConfig {
	s := map[string]string{"workspace_id": "ws"}
	if secret != "" {
		s[models.ConnectorSettingWebhookToken] = secret
	}
	return models.ConnectorConfig{Type: models.ConnectorTypeTelegram, Enabled: true, Settings: s}
}

func attentionIDs(v exposureView) []string {
	var ids []string
	for _, a := range v.Attention {
		ids = append(ids, a.ID)
	}
	return ids
}

func hasAttention(v exposureView, id string) bool {
	for _, got := range attentionIDs(v) {
		if got == id {
			return true
		}
	}
	return false
}

func TestIsLoopbackBind(t *testing.T) {
	for bind, want := range map[string]bool{
		"127.0.0.1:4001": true, "localhost:4001": true, "[::1]:4001": true, "127.1.2.3:80": true,
		"0.0.0.0:4001": false, ":4001": false, "[::]:4001": false, "192.168.1.5:4001": false, "": false,
	} {
		if got := isLoopbackBind(bind); got != want {
			t.Errorf("isLoopbackBind(%q) = %v, want %v", bind, got, want)
		}
	}
}

func TestExposure_ClosedDownInstallNeedsNothing(t *testing.T) {
	v := buildExposure(exposureInput{Bind: "127.0.0.1:4001", Encrypted: false, Sandbox: true})
	if v.Listening.Network || len(v.Attention) != 0 {
		t.Errorf("loopback is fine: %+v", v)
	}
}

func TestExposure_NetworkOnPlainHTTPIsFlaggedWithAFix(t *testing.T) {
	v := buildExposure(exposureInput{Bind: "0.0.0.0:4001", Sandbox: true})
	if !hasAttention(v, "unencrypted") {
		t.Errorf("attention = %v", attentionIDs(v))
	}
	for _, a := range v.Attention {
		if a.Fix == "" {
			t.Errorf("%s names no fix", a.ID)
		}
	}
	if len(buildExposure(exposureInput{Bind: "0.0.0.0:4001", Encrypted: true, InferenceKeyRequired: true, Sandbox: true}).Attention) != 0 {
		t.Error("an encrypted server with an inference key needs no item")
	}
}

func TestExposure_InferenceOpenOnlyMattersOnTheNetwork(t *testing.T) {
	open := buildExposure(exposureInput{Bind: "0.0.0.0:4001", Encrypted: true, Sandbox: true})
	if !hasAttention(open, "model_proxy_open") {
		t.Errorf("an open /v1 on the network should be flagged: %v", attentionIDs(open))
	}
	local := buildExposure(exposureInput{Bind: "127.0.0.1:4001", Sandbox: true})
	if hasAttention(local, "model_proxy_open") {
		t.Error("loopback: nobody else can reach /v1")
	}
	keyed := buildExposure(exposureInput{Bind: "0.0.0.0:4001", Encrypted: true, InferenceKeyRequired: true, Sandbox: true})
	if hasAttention(keyed, "model_proxy_open") {
		t.Error("a key is required, nothing to flag")
	}
}

func TestExposure_WebhookSecretsCountOnlyInboundConnectors(t *testing.T) {
	conns := map[string]models.ConnectorConfig{
		"a":    inboundConnector("s"),
		"b":    inboundConnector(""),
		"send": {Type: models.ConnectorTypeTelegram, Enabled: true, Settings: map[string]string{"chat_id": "1"}},
		"off":  {Type: models.ConnectorTypeTelegram, Settings: map[string]string{"workspace_id": "ws"}},
	}
	v := buildExposure(exposureInput{Bind: "127.0.0.1:4001", Sandbox: true, Connectors: conns})
	if v.Webhooks.Inbound != 2 || v.Webhooks.WithSecret != 1 {
		t.Errorf("webhooks = %+v, want 1 of 2", v.Webhooks)
	}
	if !hasAttention(v, "webhook_no_secret") {
		t.Errorf("a connector without a secret receives nothing: %v", attentionIDs(v))
	}
}

func TestExposure_SandboxOffIsFlagged(t *testing.T) {
	v := buildExposure(exposureInput{Bind: "127.0.0.1:4001"})
	if !hasAttention(v, "sandbox_off") {
		t.Errorf("sandbox off: %v", attentionIDs(v))
	}
}
