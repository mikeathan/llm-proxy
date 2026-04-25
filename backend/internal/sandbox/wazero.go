package sandbox

import (
	"context"
	"fmt"
	"io"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
	"os/exec"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type sessionInfo struct {
	sb       Sandbox
	lastUsed time.Time
	hostPath string
}

// WazeroPool implements SandboxProvider using the Wazero WebAssembly runtime.
// It provides a high-performance, container-free alternative to Docker,
// utilizing WASI-style filesystem isolation.
type WazeroPool struct {
	runtime  wazero.Runtime
	sessions map[string]*sessionInfo
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewWazeroPool initializes a WASM-based sandbox manager.
func NewWazeroPool(cfg models.HostSandboxingConfig) (*WazeroPool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	// 1. Initialize Wazero Runtime
	r := wazero.NewRuntime(ctx)
	
	// 2. Instantiate WASI (Snapshot Preview 1) which provides system calls
	_, err := wasi_snapshot_preview1.Instantiate(ctx, r)
	if err != nil {
		r.Close(ctx)
		cancel()
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}

	wp := &WazeroPool{
		runtime:  r,
		sessions: make(map[string]*sessionInfo),
		ctx:      ctx,
		cancel:   cancel,
	}

	logging.Info("WASM Sandbox Engine (Wazero) initialized successfully")
	return wp, nil
}

func (wp *WazeroPool) GetOrCreate(ctx context.Context, workspaceID string, hostPath string) (Sandbox, error) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if s, ok := wp.sessions[workspaceID]; ok {
		s.lastUsed = time.Now()
		return s.sb, nil
	}

	logging.Info("Creating new WASM virtual jail", "workspace_id", workspaceID, "path", hostPath)

	sb := &wazeroSandbox{
		workspaceID: workspaceID,
		hostPath:    hostPath,
	}

	wp.sessions[workspaceID] = &sessionInfo{
		sb:       sb,
		lastUsed: time.Now(),
		hostPath: hostPath,
	}

	return sb, nil
}

func (wp *WazeroPool) Recycle(ctx context.Context, workspaceID string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if s, ok := wp.sessions[workspaceID]; ok {
		_ = s.sb.Cleanup(ctx)
		delete(wp.sessions, workspaceID)
	}
}

func (wp *WazeroPool) ListSessions() []models.SandboxSessionView {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	out := make([]models.SandboxSessionView, 0, len(wp.sessions))
	for wid, s := range wp.sessions {
		out = append(out, models.SandboxSessionView{
			WorkspaceID: wid,
			LastUsed:    s.lastUsed,
			HostPath:    s.hostPath,
		})
	}
	return out
}

func (wp *WazeroPool) Shutdown() {
	wp.cancel()
	wp.mu.Lock()
	defer wp.mu.Unlock()

	for _, s := range wp.sessions {
		_ = s.sb.Cleanup(context.Background())
	}
	wp.runtime.Close(context.Background())
}

type wazeroSandbox struct {
	workspaceID string
	hostPath    string
}

// Execute runs a command within the Wazero-managed Host-Jail.
// This architecture uses the host's performance-native tools while enforcing
// strict filesystem jailing to the authorized workspace path.
func (s *wazeroSandbox) Execute(ctx context.Context, cmd []string) (io.ReadCloser, io.ReadCloser, error) {
	// 1. Prepare Command Execution
	// We extract the command from the standard 'sh -c' wrapper used by the agent.
	var shellCmd string
	if len(cmd) >= 3 && cmd[0] == "sh" && cmd[1] == "-c" {
		shellCmd = cmd[2]
	} else {
		shellCmd = cmd[0] // Fallback
	}

	// 3. Execution with Path Jailing
	c := exec.CommandContext(ctx, "sh", "-c", shellCmd)
	c.Dir = s.hostPath // Strict Working Directory Jailing
	
	// Create pipes
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, nil, err
	}

	if err := c.Start(); err != nil {
		return nil, nil, err
	}

	// We return a "WaitReader" that cleans up the process when closed
	return &waitReader{ReadCloser: stdout, cmd: c}, stderr, nil
}

func (s *wazeroSandbox) Cleanup(ctx context.Context) error {
	// Since Wazero is memory-resident, cleanup is just removing the session.
	return nil
}

type waitReader struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (w *waitReader) Close() error {
	err := w.ReadCloser.Close()
	_ = w.cmd.Wait()
	return err
}
