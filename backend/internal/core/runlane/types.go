package runlane

import (
	"context"
	"errors"
	"time"
)

// LaneKey selects the workload-class lane a run is admitted to.
type LaneKey string

const (
	LaneLocal LaneKey = "local"
	LaneCloud LaneKey = "cloud"
)

// Kind distinguishes preemptible automation runs, interactive chat runs, and
// inbound external requests (a /v1 caller waiting for the local model).
type Kind string

const (
	KindAutomation  Kind = "automation"
	KindInteractive Kind = "interactive"
	KindInbound     Kind = "inbound"
)

// Limits holds the per-lane concurrency limits. Values below 1 are clamped to 1.
type Limits struct{ Local, Cloud int }

// Disposition reports how a Submit was admitted.
type Disposition string

const (
	DispositionStarted Disposition = "started"
	DispositionQueued  Disposition = "queued"
)

var (
	ErrAlreadyQueued  = errors.New("run already queued")
	ErrClosed         = errors.New("run scheduler closed")
	ErrNotStarted     = errors.New("run scheduler not started")
	ErrPreemptTimeout = errors.New("preempted run did not stop in time")
	ErrUnknownLane    = errors.New("unknown lane")
	// ErrModelWaitCancelled is returned to a waiting inbound caller whose entry
	// was dropped (client cancel, operator dismiss) or whose wait expired.
	ErrModelWaitCancelled = errors.New("model wait cancelled")
	// ErrUnknownWaiter is returned when no queued inbound caller matches a key.
	ErrUnknownWaiter = errors.New("unknown model waiter")
)

// ModelClaim is one caller asking to serve a model: who wants what, and which
// served model that would evict. It travels as a unit because every residency
// decision needs all four values.
type ModelClaim struct {
	Active    string // local model currently served ("" when none)
	Requested string // model the caller needs
	Key       string // caller identity: a run key, or an inbound caller's key
	Label     string // human label for the operator UI (inbound callers)
}

// ResidencyResult is the verdict on switching the served local model. BlockedBy
// names the admitted run that currently uses the model an eviction would stop —
// it is the reason a switch waits or is refused, and what the UI shows the
// operator.
type ResidencyResult struct {
	Allowed   bool
	BlockedBy *Holder // nil when Allowed
}

// Job describes one schedulable run.
type Job struct {
	Key         string // "<workspace>/<automation>" — dedupe + cancel handle
	LaneKey     LaneKey
	WorkspaceID string
	Automation  string
	Label       string
	Kind        Kind
	Manual      bool // user-initiated: dropped on preemption instead of re-queued
	Model       string
	Run         func(ctx context.Context) error
}

// Submission reports the admission outcome and the 1-based queue position
// (0 when started), computed inside the enqueue critical section.
type Submission struct {
	Disposition Disposition
	Position    int
}

// Snapshot is a read model of all lanes for the API/UI. ModelHolders and
// ModelWaiters carry inbound callers: they occupy no lane slot (they use, or
// wait on, a model rather than lane concurrency), so they are reported
// separately from the per-lane state.
type Snapshot struct {
	Lanes        []LaneState
	ModelHolders []Holder
	ModelWaiters []Entry
}

// LaneState describes one workload-class lane.
type LaneState struct {
	Lane    LaneKey
	Limit   int
	Running int
	Waiting int // interactive claims waiting for a slot
	Holders []Holder
	Queued  []Entry
}

// Holder identifies a run currently occupying a lane slot. Model is the local
// model the run is using (empty for cloud runs) — the residency gate keys on it
// to refuse evicting a model out from under a live run.
//
// These tags are the wire contract with the frontend's LaneHolder; without them
// Go would emit the Go field names (WorkspaceID, Since) and the UI would read
// undefined.
type Holder struct {
	Key         string    `json:"key"`
	Kind        Kind      `json:"kind"`
	WorkspaceID string    `json:"workspace_id"`
	Automation  string    `json:"automation,omitempty"`
	Label       string    `json:"label"`
	Model       string    `json:"model,omitempty"`
	Since       time.Time `json:"since"`
}

// Entry identifies a run waiting in a lane queue, or an inbound caller waiting
// for the local model; Position is 1-based in its own queue. Tagged for the
// frontend's QueuedRun (see Holder).
type Entry struct {
	Key         string    `json:"key"`
	Kind        Kind      `json:"kind"`
	Lane        LaneKey   `json:"lane,omitempty"`
	WorkspaceID string    `json:"workspace_id"`
	Automation  string    `json:"automation,omitempty"`
	Label       string    `json:"label"`
	Model       string    `json:"model,omitempty"`
	Position    int       `json:"position"`
	QueuedAt    time.Time `json:"queued_at"`
}
