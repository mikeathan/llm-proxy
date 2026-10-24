package app

import (
	"testing"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/commands"
)

// Each helper problem reads as the reason a turn command gives for running without helpers.
func TestHelperProblemText(t *testing.T) {
	cases := map[assistant.HelperProblem]string{
		assistant.HelperNone:     "",
		assistant.HelperUnset:    "no Helper model is chosen in Settings → Runs",
		assistant.HelperMissing:  "the Helper model gone is no longer in your models",
		assistant.HelperNotCloud: "the Helper model gone is not a cloud model",
	}
	for problem, want := range cases {
		if got := helperProblemText(problem, "gone"); got != want {
			t.Errorf("%q: got %q, want %q", problem, got, want)
		}
	}
}

// The dispatcher's trigger statuses read as /run's.
func TestStartStatus(t *testing.T) {
	cases := map[string]string{
		automation.TriggerStarted: commands.AutomationStarted,
		automation.TriggerQueued:  commands.AutomationQueued,
		automation.TriggerSkipped: commands.AutomationSkipped,
	}
	for in, want := range cases {
		if got := startStatus(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
