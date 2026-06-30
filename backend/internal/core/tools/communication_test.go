package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCommunicationTools_AddConnector(t *testing.T) {
	ct := NewCommunicationTools()
	if len(ct.connectors) != 0 {
		t.Fatal("expected empty connectors map")
	}

	ct.AddConnector("test", "telegram", &TelegramNotifier{client: http.DefaultClient})
	if len(ct.connectors) != 1 {
		t.Fatal("expected 1 connector after add")
	}
}

func TestCommunicationTools_NotifyAll_Empty(t *testing.T) {
	ct := NewCommunicationTools()
	err := ct.NotifyAll(context.Background(), "hello", "")
	if err != nil {
		t.Fatalf("expected no error for empty connectors, got: %v", err)
	}
}

func TestCommunicationTools_NotifyAll_PartialFailure(t *testing.T) {
	ct := NewCommunicationTools()
	ct.AddConnector("fail", "telegram", &TelegramNotifier{client: http.DefaultClient}) // no token → error
	ct.AddConnector("fail2", "telegram", &TelegramNotifier{client: http.DefaultClient})

	err := ct.NotifyAll(context.Background(), "test", "")
	if err == nil {
		t.Fatal("expected error for misconfigured connectors")
	}
	if !strings.Contains(err.Error(), "fail") || !strings.Contains(err.Error(), "fail2") {
		t.Fatalf("expected both connector names in error, got: %v", err)
	}
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

	// Replace the API URL by constructing a notifier that points to our test server.
	// We can't inject the URL directly, but we can intercept via the client.
	n := &TelegramNotifier{
		Token:  "test-token",
		ChatID: "12345",
		client: srv.Client(),
	}
	// Override the URL construction by using a transport that rewrites the host.
	// Simpler: just test with the real Telegram URL format but intercept via client.
	// Actually, we test the Send logic by intercepting at the client level.
	// The URL is hardcoded, so we test the HTTP interaction pattern.
	// Use a custom transport that routes all requests to the test server.
	oldTransport := srv.Client().Transport
	srv.Client().Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		// Rewrite URL to test server
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		if oldTransport != nil {
			return oldTransport.RoundTrip(req)
		}
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
		{"nil client", "token", "12345"}, // client field zero value
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

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCommunicationTools_NotifyAll_AllSuccess(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ct := NewCommunicationTools()
	for i := 0; i < 3; i++ {
		n := &TelegramNotifier{
			Token:  fmt.Sprintf("token-%d", i),
			ChatID: "12345",
			client: srv.Client(),
		}
		n.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		})
		ct.AddConnector(fmt.Sprintf("conn-%d", i), "telegram", n)
	}

	err := ct.NotifyAll(context.Background(), "broadcast", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got %d", callCount)
	}
}

func TestCommunicationTools_NotifyAll_FilterByType(t *testing.T) {
	ct := NewCommunicationTools()
	ct.AddConnector("tg-1", "telegram", &TelegramNotifier{Token: "a", ChatID: "1", client: http.DefaultClient})
	ct.AddConnector("tg-2", "telegram", &TelegramNotifier{Token: "b", ChatID: "2", client: http.DefaultClient})
	ct.AddConnector("sl-1", "slack", &TelegramNotifier{Token: "", ChatID: "", client: http.DefaultClient}) // empty token = fast fail

	// Filter by type "slack" — only sl-1 should be attempted
	err := ct.NotifyAll(context.Background(), "test", "slack")
	if err == nil {
		t.Fatal("expected error because sl-1 has no real client")
	}
	// Error should reference sl-1, not tg-1 or tg-2
	if !strings.Contains(err.Error(), "sl-1") {
		t.Fatalf("expected sl-1 in error, got: %v", err)
	}
	if strings.Contains(err.Error(), "tg-1") || strings.Contains(err.Error(), "tg-2") {
		t.Fatal("telegram connectors should not be called with slack filter")
	}
}

func TestCommunicationTools_NotifyAll_FilterNoMatch(t *testing.T) {
	ct := NewCommunicationTools()
	ct.AddConnector("tg", "telegram", &TelegramNotifier{client: http.DefaultClient})

	err := ct.NotifyAll(context.Background(), "test", "slack")
	if err == nil {
		t.Fatal("expected error when filter matches no connectors")
	}
	if !strings.Contains(err.Error(), "no connector found for type 'slack'") {
		t.Fatalf("expected 'no connector found' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "telegram") {
		t.Fatalf("expected available types list in error, got: %v", err)
	}
}

func TestCommunicationTools_NotifyAll_FilterEmptyIsBroadcast(t *testing.T) {
	ct := NewCommunicationTools()
	ct.AddConnector("tg", "telegram", &TelegramNotifier{Token: "", ChatID: "", client: http.DefaultClient})
	ct.AddConnector("sl", "slack", &TelegramNotifier{Token: "", ChatID: "", client: http.DefaultClient})

	// empty connectorType = broadcast to all — both should be attempted
	err := ct.NotifyAll(context.Background(), "test", "")
	if err == nil {
		t.Fatal("expected error because both have no real client")
	}
	if !strings.Contains(err.Error(), "tg") || !strings.Contains(err.Error(), "sl") {
		t.Fatalf("expected both connectors in error, got: %v", err)
	}
}
