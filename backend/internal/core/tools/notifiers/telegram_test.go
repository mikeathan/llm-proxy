package notifiers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTelegramNotifier_Send_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("expected form content-type, got %s", r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("chat_id") != "12345" {
			t.Fatalf("expected chat_id=12345, got %s", r.Form.Get("chat_id"))
		}
		if r.Form.Get("text") != "hello" {
			t.Fatalf("expected text=hello, got %s", r.Form.Get("text"))
		}
		if r.Form.Get("parse_mode") != "Markdown" {
			t.Fatalf("expected parse_mode=Markdown, got %s", r.Form.Get("parse_mode"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &TelegramNotifier{
		Token:  "test-token",
		ChatID: "12345",
		client: srv.Client(),
	}
	n.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	err := n.Send(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestTelegramNotifier_Send_MissingCredentials(t *testing.T) {
	tests := []struct {
		name  string
		token string
		chat  string
	}{
		{"empty token", "", "12345"},
		{"empty chat", "token", ""},
		{"both empty", "", ""},
		{"nil client", "token", "12345"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := &TelegramNotifier{Token: tc.token, ChatID: tc.chat}
			err := n.Send(context.Background(), "msg")
			if err == nil {
				t.Fatal("expected error for missing credentials")
			}
		})
	}
}

func TestTelegramNotifier_Send_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer srv.Close()

	n := &TelegramNotifier{
		Token:  "bad-token",
		ChatID: "12345",
		client: srv.Client(),
	}
	n.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	err := n.Send(context.Background(), "msg")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 in error, got: %v", err)
	}
}

func TestTelegramNotifier_RegisterWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("url") != "https://example.com/webhook/tg-1" {
			t.Fatalf("expected url=https://example.com/webhook/tg-1, got %s", r.Form.Get("url"))
		}
		if r.Form.Get("allowed_updates") != `["message"]` {
			t.Fatalf("expected allowed_updates=[\"message\"], got %s", r.Form.Get("allowed_updates"))
		}
		if r.Form.Get("secret_token") != "sec-123" {
			t.Fatalf("expected secret_token=sec-123, got %s", r.Form.Get("secret_token"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true,"result":true,"description":"Webhook was set"}`))
	}))
	defer srv.Close()

	n := &TelegramNotifier{Token: "test-token", ChatID: "12345", client: srv.Client()}
	n.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	err := n.RegisterWebhook(context.Background(), "https://example.com/webhook/tg-1", "sec-123")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestTelegramNotifier_RegisterWebhook_NoSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("secret_token") != "" {
			t.Fatalf("expected no secret_token, got %s", r.Form.Get("secret_token"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	n := &TelegramNotifier{Token: "test-token", ChatID: "12345", client: srv.Client()}
	n.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	err := n.RegisterWebhook(context.Background(), "https://example.com/webhook/tg-1", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestTelegramNotifier_RegisterWebhook_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":false,"description":"Not authorized"}`))
	}))
	defer srv.Close()

	n := &TelegramNotifier{Token: "bad-token", ChatID: "12345", client: srv.Client()}
	n.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	err := n.RegisterWebhook(context.Background(), "https://example.com/w", "")
	if err == nil {
		t.Fatal("expected error for API rejection")
	}
	if !strings.Contains(err.Error(), "Not authorized") {
		t.Fatalf("expected 'Not authorized' in error, got: %v", err)
	}
}

func TestTelegramNotifier_GetWebhookInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true,"result":{"url":"https://example.com/w","pending_update_count":3,"last_error_message":"connection timeout"}}`))
	}))
	defer srv.Close()

	n := &TelegramNotifier{Token: "test-token", ChatID: "12345", client: srv.Client()}
	n.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	info, err := n.GetWebhookInfo(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if info.URL != "https://example.com/w" {
		t.Fatalf("expected URL https://example.com/w, got %s", info.URL)
	}
	if info.PendingUpdates != 3 {
		t.Fatalf("expected 3 pending updates, got %d", info.PendingUpdates)
	}
	if info.LastError != "connection timeout" {
		t.Fatalf("expected 'connection timeout' last error, got %s", info.LastError)
	}
}

func TestTelegramNotifier_GetWebhookInfo_NoWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true,"result":{"url":"","pending_update_count":0,"last_error_message":""}}`))
	}))
	defer srv.Close()

	n := &TelegramNotifier{Token: "test-token", ChatID: "12345", client: srv.Client()}
	n.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	info, err := n.GetWebhookInfo(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if info.URL != "" {
		t.Fatalf("expected empty URL for no webhook, got %s", info.URL)
	}
}

func TestTelegramNotifier_DeleteWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true,"result":true,"description":"Webhook was deleted"}`))
	}))
	defer srv.Close()

	n := &TelegramNotifier{Token: "test-token", ChatID: "12345", client: srv.Client()}
	n.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	err := n.DeleteWebhook(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestTelegramNotifier_WebhookMethods_MissingCredentials(t *testing.T) {
	t.Run("RegisterWebhook empty token", func(t *testing.T) {
		n := &TelegramNotifier{Token: "", ChatID: "12345", client: http.DefaultClient}
		err := n.RegisterWebhook(context.Background(), "https://example.com/w", "")
		if err == nil {
			t.Fatal("expected error for empty token")
		}
	})
	t.Run("GetWebhookInfo empty token", func(t *testing.T) {
		n := &TelegramNotifier{Token: "", ChatID: "12345", client: http.DefaultClient}
		_, err := n.GetWebhookInfo(context.Background())
		if err == nil {
			t.Fatal("expected error for empty token")
		}
	})
	t.Run("DeleteWebhook nil client", func(t *testing.T) {
		n := &TelegramNotifier{Token: "token", ChatID: "12345"}
		err := n.DeleteWebhook(context.Background())
		if err == nil {
			t.Fatal("expected error for nil client")
		}
	})
}

func TestTelegramFactoryRegistered(t *testing.T) {
	factory, ok := tools.GetConnectorFactory(models.ConnectorTypeTelegram)
	if !ok {
		t.Fatal("expected telegram connector factory to be registered via init()")
	}

	secrets := &fakeSecrets{creds: map[string]string{"connector:my-tg": "bot-token"}}
	cfg := models.ConnectorConfig{
		Type:     models.ConnectorTypeTelegram,
		Settings: map[string]string{"chat_id": "12345"},
	}
	conn, ok := factory("my-tg", cfg, secrets, tools.NewNetworkTools(nil, nil))
	if !ok {
		t.Fatal("expected factory to build connector with valid credentials")
	}
	if conn.Name() != "Telegram" {
		t.Errorf("unexpected connector name: %q", conn.Name())
	}

	// Missing credentials -> ok=false
	badCfg := models.ConnectorConfig{Type: models.ConnectorTypeTelegram, Settings: map[string]string{}}
	if _, ok := factory("my-tg", badCfg, secrets, tools.NewNetworkTools(nil, nil)); ok {
		t.Error("expected factory to reject missing chat_id")
	}
}

type fakeSecrets struct {
	creds map[string]string
}

func (f *fakeSecrets) GetSecret(category, provider string) string {
	return f.creds[category+":"+provider]
}
func (f *fakeSecrets) SetSecret(category, provider, value string) error       { return nil }
func (f *fakeSecrets) MaskedSecret(category, provider string) string           { return "" }
func (f *fakeSecrets) GetProviderKeys(provider string) []models.APIKeyItem     { return nil }
func (f *fakeSecrets) SetProviderKeys(provider string, keys []models.APIKeyItem) error { return nil }
func (f *fakeSecrets) DeleteProviderKey(provider, keyID string) error         { return nil }
func (f *fakeSecrets) DeleteAllProviderKeys(provider string) error            { return nil }
func (f *fakeSecrets) MaskedProviderKeys(provider string) []models.APIKeyItem { return nil }
func (f *fakeSecrets) GetResolvedProviderKey(provider, name string) (string, error) { return "", nil }
func (f *fakeSecrets) GetResolvedProviderKeyInfo(provider, name string) (*models.ResolvedProviderKeyInfo, error) {
	return nil, nil
}
func (f *fakeSecrets) ResolveMaskedKey(provider, maskedKey string) (string, error) { return "", nil }
