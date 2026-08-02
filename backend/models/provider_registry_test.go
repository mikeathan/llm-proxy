package models

import "testing"

// TestProviderRegistryCoversTuningTable ensures the numeric tuning table has a
// row for every canonical provider and no stray keys.
func TestProviderRegistryCoversTuningTable(t *testing.T) {
	tuning := ProviderTuningDefaults()
	ids := ProviderIDs()

	for _, id := range ids {
		if _, ok := tuning[id]; !ok {
			t.Errorf("provider %q missing from tuning table", id)
		}
	}
	for k := range tuning {
		found := false
		for _, id := range ids {
			if id == k {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tuning table has key %q not in ProviderIDs", k)
		}
	}
}

// TestSupportsBaseURLMatrix locks the capability so it cannot silently drift.
func TestSupportsBaseURLMatrix(t *testing.T) {
	want := map[string]bool{
		ProviderLocal:      false,
		ProviderGemini:     false,
		ProviderOpenAI:     true,
		ProviderOpenRouter: true,
		ProviderNVIDIA:     true,
	}
	for id, exp := range want {
		if got := SupportsBaseURL(id); got != exp {
			t.Errorf("SupportsBaseURL(%q) = %v, want %v", id, got, exp)
		}
	}
}
