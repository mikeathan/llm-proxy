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
}
