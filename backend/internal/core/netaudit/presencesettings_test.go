package netaudit

import (
	"errors"
	"testing"
	"time"

	"llm-proxy/models"
)

type memConfig struct{ cfg models.NetauditConfig }

func (m *memConfig) Get() models.NetauditConfig { return m.cfg }
func (m *memConfig) Update(f func(*models.NetauditConfig) error) error {
	next := m.cfg
	if err := f(&next); err != nil {
		return err
	}
	m.cfg = next
	return nil
}

func TestPresenceConfig_IsOffByDefaultAndEveryTenMinutes(t *testing.T) {
	c := NewPresenceConfig(&memConfig{})
	view := c.View()
	if view.Enabled || view.IntervalMinutes != DefaultPresenceInterval {
		t.Errorf("view = %+v", view)
	}
	if got := c.Settings(); got.Enabled || got.Every != 10*time.Minute {
		t.Errorf("settings = %+v", got)
	}
}

func TestPresenceConfig_SetValidatesTheInterval(t *testing.T) {
	store := &memConfig{}
	c := NewPresenceConfig(store)
	if err := c.Set(true, 15); err != nil {
		t.Fatal(err)
	}
	if got := c.View(); !got.Enabled || got.IntervalMinutes != 15 || store.cfg.PresenceIntervalMinutes != 15 {
		t.Errorf("view = %+v", got)
	}
	if err := c.Set(true, 3); !errors.Is(err, ErrBadPresenceInterval) {
		t.Errorf("3 minutes is not a choice: %v", err)
	}
	if got := c.View(); got.IntervalMinutes != 15 {
		t.Errorf("a refused change must change nothing: %+v", got)
	}
	if err := c.Set(false, 0); err != nil || c.View().Enabled || c.View().IntervalMinutes != DefaultPresenceInterval {
		t.Errorf("0 means the default, and false turns it off: %+v %v", c.View(), err)
	}
}

func TestPresenceConfig_TargetsAreTheDeclaredRanges(t *testing.T) {
	store := &memConfig{cfg: models.NetauditConfig{ScanRanges: []string{"192.168.1.0/24"}, PresenceCheck: true}}
	got := NewPresenceConfig(store).Settings()
	if !got.Enabled || len(got.Targets) != 1 || got.Targets[0] != "192.168.1.0/24" {
		t.Errorf("settings = %+v", got)
	}
}
