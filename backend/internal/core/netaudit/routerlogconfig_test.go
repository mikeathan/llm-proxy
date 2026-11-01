package netaudit

import (
	"testing"

	"llm-proxy/models"
)

type memPolicy struct{ cfg models.NetauditConfig }

func (m *memPolicy) Get() models.NetauditConfig { return m.cfg }
func (m *memPolicy) Update(fn func(*models.NetauditConfig) error) error {
	return fn(&m.cfg)
}

func TestRouterLogConfig_IsOffUntilTurnedOn(t *testing.T) {
	store := &memPolicy{}
	c := NewRouterLogConfig(store)
	if c.Enabled() {
		t.Fatal("the listener must be off by default")
	}
	if err := c.Set(true); err != nil || !c.Enabled() {
		t.Fatalf("Set(true): %v, enabled %v", err, c.Enabled())
	}
	if err := c.Set(false); err != nil || c.Enabled() {
		t.Fatalf("Set(false): %v, enabled %v", err, c.Enabled())
	}
}
