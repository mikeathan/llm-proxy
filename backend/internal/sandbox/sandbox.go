package sandbox

import (
	"context"
	"io"
)

// Sandbox defines the contract for an isolated execution environment.
type Sandbox interface {
	// Execute runs a command inside the sandbox and returns stdout/stderr streams.
	Execute(ctx context.Context, cmd []string) (stdout io.ReadCloser, stderr io.ReadCloser, err error)

	// Cleanup destroys the sandbox securely and frees resources.
	Cleanup(ctx context.Context) error
}
