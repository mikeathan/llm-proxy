// Package procwatch provides a single-owner wrapper around exec.Cmd lifecycle
// tracking. It centralizes the "wait for process exit via a done channel"
// pattern so callers never call cmd.Wait() more than once (a second Wait
// returns "exec: Wait was already called").
package procwatch

import (
	"os/exec"
	"sync"
)

// Watch tracks an *exec.Cmd's lifecycle with a single owner of Cmd.Wait. It
// detects unexpected process exit (e.g. a server that crashes on launch) and
// exposes a done channel for shutdown paths to wait on.
type Watch struct {
	done chan struct{}
	mu   sync.Mutex
	err  error
}

// New launches a goroutine that calls cmd.Wait() and closes the done channel
// when the process terminates. The caller must not call cmd.Wait() themselves.
func New(cmd *exec.Cmd) *Watch {
	w := &Watch{done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		w.mu.Lock()
		w.err = err
		w.mu.Unlock()
		close(w.done)
	}()
	return w
}

// Done returns the process-exit channel; it is closed once the process has
// terminated.
func (w *Watch) Done() <-chan struct{} {
	return w.done
}

// Exited reports whether the process has terminated.
func (w *Watch) Exited() bool {
	select {
	case <-w.done:
		return true
	default:
		return false
	}
}

// Err returns the process exit error if the process has terminated, else nil.
func (w *Watch) Err() error {
	select {
	case <-w.done:
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.err
	default:
		return nil
	}
}
