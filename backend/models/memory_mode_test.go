package models

import "testing"

func TestMemoryMode_Valid(t *testing.T) {
	cases := map[MemoryMode]bool{
		MemoryModeOn:  true,
		MemoryModeOff: true,
		"":            false, // inherit is resolved by Effective before Valid is asked
		"hot":         false, // the pre-rename value is not accepted
		"hot+hints":   false, // not shipped: step-aware hints wait for a measured win
		"always":      false,
	}
	for mode, want := range cases {
		if got := mode.Valid(); got != want {
			t.Errorf("MemoryMode(%q).Valid() = %v, want %v", mode, got, want)
		}
	}
}

func TestMemoryMode_Effective(t *testing.T) {
	cases := []struct {
		mode          MemoryMode
		globalDefault bool
		want          bool
	}{
		{MemoryModeInherit, true, true},
		{MemoryModeInherit, false, false},
		{MemoryModeOn, false, true},
		{MemoryModeOn, true, true},
		{MemoryModeOff, true, false},
		{MemoryModeOff, false, false},
		{"hot", true, true}, // an unreadable stale value reads as inherit, never as an override
		{"hot", false, false},
	}
	for _, c := range cases {
		if got := c.mode.Effective(c.globalDefault); got != c.want {
			t.Errorf("MemoryMode(%q).Effective(%v) = %v, want %v", c.mode, c.globalDefault, got, c.want)
		}
	}
}

func TestUnattendedRun_ContextFlag(t *testing.T) {
	ctx := t.Context()
	if IsUnattendedRun(ctx) {
		t.Error("a bare context is not an unattended run")
	}
	if !IsUnattendedRun(WithUnattendedRun(ctx)) {
		t.Error("WithUnattendedRun must mark the context")
	}
	if IsUnattendedRun(nil) { //nolint:staticcheck // nil-safety is part of the contract (GetRunID etc.)
		t.Error("nil context is not unattended")
	}
}
