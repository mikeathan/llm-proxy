package app

import (
	"context"
	"slices"
	"testing"

	"llm-proxy-stacks/radio"
	"llm-proxy/internal/core/radiodigest"
	"llm-proxy/models"
)

// Without a running Radio stack the tool is not offered and a call is an error: a stack that cannot answer must never read as a quiet air.
func TestRadioDigestTool_IsHiddenAndRefusesWithoutARunningRadioStack(t *testing.T) {
	var s AppServices
	if s.radioOn(context.Background()) {
		t.Error("the tool must not be offered without a radio stack")
	}
	if raw, err := s.radioDigest(context.Background()); err == nil {
		t.Errorf("a missing stack must be an error, got %s", raw)
	}
}

func TestRadioDigestTool_IsAReadOnlyToolOfTheFindingsCategory(t *testing.T) {
	if got := models.CategoryOf(models.ToolRadioDigest); got != models.CategoryNetFindings {
		t.Errorf("category = %q, want %q (profiles that read findings get it, the unattended auditor does not)", got, models.CategoryNetFindings)
	}
}

// The core words each finding from its own table; the stack's ids and words are held to it here, so a check added to the stack without words in the
// core is a failing test and not a finding the model never hears about.
func TestRadioDigestTool_TheCoreHasWordsForEveryFindingTheStackOffers(t *testing.T) {
	core, stk := radiodigest.FindingIDs(), radio.FindingIDs()
	slices.Sort(core)
	slices.Sort(stk)
	if !slices.Equal(core, stk) {
		t.Fatalf("the core words %v, the stack offers %v", core, stk)
	}
	for id, theirs := range radio.FindingWords() {
		if ours, _, _ := radiodigest.Words(id); ours != theirs {
			t.Errorf("%s: the core says %q, the stack says %q", id, ours, theirs)
		}
	}
	for id, theirs := range radio.FindingMeanings() {
		if _, ours, _ := radiodigest.Words(id); ours != theirs {
			t.Errorf("%s: the core's meaning line says %q, the stack's says %q", id, ours, theirs)
		}
	}
}
