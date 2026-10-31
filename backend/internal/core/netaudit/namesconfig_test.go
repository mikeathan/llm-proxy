package netaudit

import "testing"

func TestNamesConfig_IsOnByDefaultAndOffOnlyWhenSwitchedOff(t *testing.T) {
	store := &memConfig{}
	c := NewNamesConfig(store)
	if !c.Enabled() {
		t.Fatal("nothing stored must mean on: naming is the default")
	}
	if err := c.Set(false); err != nil || c.Enabled() || !store.Get().DeviceNamesOff {
		t.Errorf("off: err=%v enabled=%v", err, c.Enabled())
	}
	if err := c.Set(true); err != nil || !c.Enabled() || store.Get().DeviceNamesOff {
		t.Errorf("on again: err=%v enabled=%v", err, c.Enabled())
	}
}

func TestSummaryConfig_IsOffByDefault(t *testing.T) {
	store := &memConfig{}
	c := NewSummaryConfig(store)
	if c.Enabled() {
		t.Fatal("the weekly summary is off until switched on")
	}
	if err := c.Set(true); err != nil || !c.Enabled() {
		t.Errorf("on: err=%v enabled=%v", err, c.Enabled())
	}
}
