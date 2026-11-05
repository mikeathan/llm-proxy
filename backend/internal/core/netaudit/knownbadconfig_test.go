package netaudit

import "testing"

func TestKnownBadConfig_IsOffUntilSwitchedOn(t *testing.T) {
	store := &memConfig{}
	c := NewKnownBadConfig(store)
	if c.Enabled() {
		t.Fatal("off until the operator turns it on")
	}
	if err := c.Set(true); err != nil || !c.Enabled() || !store.Get().KnownBadEnabled {
		t.Errorf("on: err=%v enabled=%v", err, c.Enabled())
	}
}
