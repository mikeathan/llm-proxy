package netaudit

import (
	"testing"

	"llm-proxy/internal/core/netaudit/footprint"
)

// Every footprint check has a plain title and a fix, so a finding is never shown as a bare id.
func TestFootprintChecksHaveATitleAndAFix(t *testing.T) {
	for _, check := range footprint.Checks {
		if CheckTitle(check) == "" || RemediationFor(check) == "" {
			t.Errorf("%s has no title or no fix", check)
		}
	}
}
