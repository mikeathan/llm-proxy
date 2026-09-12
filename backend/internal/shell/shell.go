package shell

import (
	"context"
	"io"
	"llm-proxy/internal/platform/sandbox"
	"llm-proxy/models"
	"time"
)

// Terminal defines the contract for a terminal execution session.
type Terminal interface {
	// Execute runs a command and returns stdout/stderr streams.
	Execute(ctx context.Context, cmd []string) (stdout io.ReadCloser, stderr io.ReadCloser, err error)

	// Cleanup destroys the session and frees resources.
	Cleanup(ctx context.Context) error

	// PGID returns a negated process group ID suitable for syscall.Kill.
	// Returns 0 if the session has no active process.
	PGID() int
}

// WorkspacePolicy captures the per-workspace, per-run properties that define a
// shell session's isolation (agent-os-sandboxing plan D8). Sessions are pooled
// per (workspaceID, NetworkOn) — network state is a process-level OS property,
// so two scopes need two shells (≤2 per workspace; the stale scope is idle-
// reaped). Epoch is reserved for recycling same-key shells when non-scope
// policy (filesystem jail, capabilities) changes; no producer increments it
// yet, so it stays 0 (the recycle branch is exercised by tests only).
type WorkspacePolicy struct {
	AllowedEnvVars []string // env allowlist for prepareShellEnv
	PathExtensions []string // PATH extensions (workspace-relative or absolute)
	NetworkOn      bool     // OS network state for child processes (D8 key)
	Epoch          int64    // policy epoch; 0 until a producer wires increments
	IdleTimeout    time.Duration
	// ProxyEnv holds HTTP(S)_PROXY/NO_PROXY entries when the host egress proxy
	// is enabled; they override the env floor for shells whose NetworkOn is true.
	ProxyEnv []string
	// Sandbox is the OS confinement provider applied to the spawned shell
	// BEFORE Start (Landlock on Linux kernels >= 5.13; no-op elsewhere — plan
	// Phase 2).
	Sandbox sandbox.Provider
}

// ShellProvider manages multiple terminal sessions across workspaces.
type ShellProvider interface {
	// GetOrCreate returns an existing session for the (workspace, policy) key or
	// creates a new one. A same-key session created under an older policy epoch
	// is recycled (Cleanup + recreate) so long-lived shells never enforce a
	// stale policy.
	GetOrCreate(ctx context.Context, workspaceID string, hostPath string, policy WorkspacePolicy) (Terminal, error)

	// ListSessions returns a view of all active sessions.
	ListSessions() []models.TerminalSessionView

	// Recycle force-restarts a session for a workspace.
	Recycle(ctx context.Context, workspaceID string)

	// Shutdown stops all active sessions and the provider.
	Shutdown(ctx context.Context)

	// PGID returns the negated process group ID for a workspace's active
	// shell session. ok=false when no active session exists.
	PGID(workspaceID string) (pgid int, ok bool)
}
