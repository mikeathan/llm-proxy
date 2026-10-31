package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

type memAlertStore struct{ items []models.NetworkAlert }

func (m *memAlertStore) Read() ([]models.NetworkAlert, error) { return m.items, nil }
func (m *memAlertStore) Write(v []models.NetworkAlert) error  { m.items = v; return nil }

// An alert reaches the log even when no connector exists, and says so: the web UI is then the only place it shows.
func TestAlertNotifier_LogsTheAlertWhenNoConnectorIsSetUp(t *testing.T) {
	store := &memAlertStore{}
	s := &AppServices{}
	err := s.alertNotifier(netaudit.NewAlertLog(store, time.Now, nil), models.NetworkAlertPresence)(context.Background(), "Network audit: 1 new device.")
	if !errors.Is(err, netaudit.ErrNoConnector) {
		t.Fatalf("the alert must stay pending, got %v", err)
	}
	if len(store.items) != 1 || store.items[0].Delivery != models.NetworkAlertNoConnector || store.items[0].Kind != models.NetworkAlertPresence {
		t.Fatalf("got %+v", store.items)
	}
}
