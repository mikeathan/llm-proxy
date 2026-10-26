package netaudit

import (
	"context"
	"strings"

	"llm-proxy/models"
)

// AuditorProfileID is the built-in profile a scheduled network audit runs under.
const AuditorProfileID = models.AuditorProfileID

// Preflight returns the check that runs before a scheduled audit touches the model. It is a refusal when anything a
// scan needs is missing — including what only limits an attended scan (nmap, file confinement), because an audit does
// not fall back to the limited engine. Automations that are not audits are not checked.
func (r *Readiness) Preflight(ctx context.Context, profileID string) (code, message string, refused bool) {
	if profileID != AuditorProfileID {
		return "", "", false
	}
	missing := r.Check(ctx)
	if len(missing) == 0 {
		return "", "", false
	}
	messages := make([]string, len(missing))
	for i, req := range missing {
		messages[i] = req.Message
	}
	return string(missing[0].Code), strings.Join(messages, " "), true
}
