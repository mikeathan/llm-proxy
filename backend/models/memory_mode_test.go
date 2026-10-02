package models

import "testing"

func TestMemoryMode_Valid(t *testing.T) {
	cases := map[MemoryMode]bool{
		MemoryModeOff: true,
		MemoryModeHot: true,
		"":            false, // callers treat empty as "unset = off" before calling Valid
		"hot+hints":   false, // not shipped: step-aware hints wait for a measured win
		"always":      false,
	}
	for mode, want := range cases {
		if got := mode.Valid(); got != want {
			t.Errorf("MemoryMode(%q).Valid() = %v, want %v", mode, got, want)
		}
	}
}

// Unset must behave as off so existing automations are unchanged.
func TestMemoryMode_HotEnabled(t *testing.T) {
	cases := map[MemoryMode]bool{"": false, MemoryModeOff: false, MemoryModeHot: true}
	for mode, want := range cases {
		if got := mode.HotEnabled(); got != want {
			t.Errorf("MemoryMode(%q).HotEnabled() = %v, want %v", mode, got, want)
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
