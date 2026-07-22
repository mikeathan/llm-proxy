package shell

import (
	"context"
	"io"
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

// ShellProvider manages multiple terminal sessions across workspaces.
type ShellProvider interface {
	// GetOrCreate returns an existing session or creates a new one for the workspace.
	GetOrCreate(ctx context.Context, workspaceID string, hostPath string, idleTimeout time.Duration, allowedEnvVars []string, pathExtensions []string) (Terminal, error)

	// ListSessions returns a view of all active sessions.
	ListSessions() []models.TerminalSessionView

	// Recycle force-restarts a session for a workspace.
	Recycle(ctx context.Context, workspaceID string)

	// Shutdown stops all active sessions and the provider.
	Shutdown()

	// PGID returns the negated process group ID for a workspace's active
	// shell session. ok=false when no active session exists.
	PGID(workspaceID string) (pgid int, ok bool)
}
