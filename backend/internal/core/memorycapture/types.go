// Package memorycapture turns what a user says in a chat into memory candidates: explicit "remember …" requests
// found by plain code (Extractor, Capture) and, in review.go, a model's proposals the user approves. It is domain
// logic only — storage and the model arrive through the ports below — so it has no HTTP, SQL or agent imports.
package memorycapture

import (
	"context"

	"llm-proxy/internal/platform/memory"
)

// Candidate is one fact proposed for memory.
type Candidate struct {
	Content string
	Scope   memory.Scope
	Mode    memory.Mode
	// Origin names what produced it: the phrase family, or "review".
	Origin string
}

// Outcome is what saving a candidate did.
type Outcome int

const (
	OutcomeCreated Outcome = iota + 1
	OutcomeUpdated
	OutcomeDuplicate
)

// Library is the memory store as this package needs it; one save path serves the tool and the capture.
type Library interface {
	Save(ctx context.Context, workspaceID string, c Candidate) (Outcome, error)
	Has(ctx context.Context, workspaceID, content string) bool
}

// Completer runs one model completion; the reviewer is its only caller.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// SecretCheck reports whether text looks like a credential.
type SecretCheck func(string) bool

func noSecrets(string) bool { return false }
