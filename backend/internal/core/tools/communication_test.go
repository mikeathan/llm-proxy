package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type stubConnector struct {
	name     string
	sendErr  error
}

func (s *stubConnector) Name() string { return s.name }

func (s *stubConnector) Send(_ context.Context, message string) error {
	if s.sendErr != nil {
		return fmt.Errorf("%s: %w", s.name, s.sendErr)
	}
	return nil
}

func TestCommunicationTools_AddConnector(t *testing.T) {
	ct := NewCommunicationTools()
	if len(ct.connectors) != 0 {
		t.Fatal("expected empty connectors map")
	}

	ct.AddConnector("test", "telegram", &stubConnector{name: "Telegram"})
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
	ct.AddConnector("fail", "telegram", &stubConnector{name: "Telegram-Fail", sendErr: fmt.Errorf("no token")})
	ct.AddConnector("fail2", "telegram", &stubConnector{name: "Telegram-Fail2", sendErr: fmt.Errorf("no chat")})

	err := ct.NotifyAll(context.Background(), "test", "")
	if err == nil {
		t.Fatal("expected error for misconfigured connectors")
	}
	if !strings.Contains(err.Error(), "fail") || !strings.Contains(err.Error(), "fail2") {
		t.Fatalf("expected both connector names in error, got: %v", err)
	}
}

func TestCommunicationTools_GetByName(t *testing.T) {
	ct := NewCommunicationTools()
	conn := &stubConnector{name: "Telegram"}
	ct.AddConnector("tg", "telegram", conn)

	got, ok := ct.GetByName("tg")
	if !ok {
		t.Fatal("expected connector to be found")
	}
	if got.Name() != "Telegram" {
		t.Fatalf("expected name 'Telegram', got %q", got.Name())
	}

	_, ok = ct.GetByName("nonexistent")
	if ok {
		t.Fatal("expected nonexistent connector to not be found")
	}
}

func TestCommunicationTools_NotifyAll_AllSuccess(t *testing.T) {
	ct := NewCommunicationTools()
	for i := 0; i < 3; i++ {
		ct.AddConnector(fmt.Sprintf("conn-%d", i), "telegram", &stubConnector{name: fmt.Sprintf("Telegram-%d", i)})
	}

	err := ct.NotifyAll(context.Background(), "broadcast", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestCommunicationTools_NotifyAll_FilterByType(t *testing.T) {
	ct := NewCommunicationTools()
	ct.AddConnector("tg-1", "telegram", &stubConnector{name: "Telegram-1"})
	ct.AddConnector("tg-2", "telegram", &stubConnector{name: "Telegram-2"})
	ct.AddConnector("sl-1", "slack", &stubConnector{name: "Slack-1", sendErr: fmt.Errorf("no token")})

	err := ct.NotifyAll(context.Background(), "test", "slack")
	if err == nil {
		t.Fatal("expected error because slack connector has no real client")
	}
	if !strings.Contains(err.Error(), "sl-1") {
		t.Fatalf("expected sl-1 in error, got: %v", err)
	}
	if strings.Contains(err.Error(), "tg-1") || strings.Contains(err.Error(), "tg-2") {
		t.Fatal("telegram connectors should not be called with slack filter")
	}
}

func TestCommunicationTools_NotifyAll_FilterNoMatch(t *testing.T) {
	ct := NewCommunicationTools()
	ct.AddConnector("tg", "telegram", &stubConnector{name: "Telegram"})

	err := ct.NotifyAll(context.Background(), "test", "slack")
	if err == nil {
		t.Fatal("expected error when filter matches no connectors")
	}
	if !strings.Contains(err.Error(), "no connector found") {
		t.Fatalf("expected 'no connector found' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "telegram") {
		t.Fatalf("expected available types list in error, got: %v", err)
	}
}

func TestCommunicationTools_NotifyAll_FilterEmptyIsBroadcast(t *testing.T) {
	ct := NewCommunicationTools()
	ct.AddConnector("tg", "telegram", &stubConnector{name: "Telegram", sendErr: fmt.Errorf("no token")})
	ct.AddConnector("sl", "slack", &stubConnector{name: "Slack", sendErr: fmt.Errorf("no token")})

	err := ct.NotifyAll(context.Background(), "test", "")
	if err == nil {
		t.Fatal("expected error because both have errors")
	}
	if !strings.Contains(err.Error(), "tg") || !strings.Contains(err.Error(), "sl") {
		t.Fatalf("expected both connectors in error, got: %v", err)
	}
}
