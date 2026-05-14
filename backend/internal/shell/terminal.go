package shell

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var defaultSystemPaths = []string{
	"/opt/homebrew/bin",
	"/opt/homebrew/sbin",
	"/usr/local/bin",
	"/usr/bin",
	"/bin",
	"/usr/sbin",
	"/sbin",
}

type persistentShell struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	done   chan struct{}
}

func newPersistentShell(ctx context.Context, hostPath string, env []string) (*persistentShell, error) {
	cmd := exec.CommandContext(ctx, "bash", "--norc", "--noprofile", "-s")
	cmd.Dir = hostPath
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Redirect stderr to the same pipe so we capture all output
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start persistent shell: %w", err)
	}

	ps := &persistentShell{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
		done:   make(chan struct{}),
	}

	go func() {
		_ = cmd.Wait()
		close(ps.done)
	}()

	return ps, nil
}

func (ps *persistentShell) Execute(ctx context.Context, command string) (string, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	sentinel := fmt.Sprintf("__SHELL_DONE_%d__", time.Now().UnixNano())

	script := fmt.Sprintf("%s; echo '%s'$?\n", command, sentinel)
	if _, err := io.WriteString(ps.stdin, script); err != nil {
		return "", fmt.Errorf("failed to write command to shell: %w", err)
	}

	var outputBuf strings.Builder
	for {
		select {
		case <-ctx.Done():
			return outputBuf.String(), ctx.Err()
		case <-ps.done:
			return outputBuf.String(), fmt.Errorf("shell process exited unexpectedly")
		default:
		}

		line, err := ps.stdout.ReadString('\n')
		if err != nil && err != io.EOF {
			return outputBuf.String(), fmt.Errorf("failed to read shell output: %w", err)
		}

		trimmed := strings.TrimSuffix(line, "\n")

		if idx := strings.Index(trimmed, sentinel); idx != -1 {
			// Append the part before the sentinel to the output
			outputBuf.WriteString(trimmed[:idx])

			exitCodeStr := trimmed[idx+len(sentinel):]
			exitCode, _ := strconv.Atoi(strings.TrimSpace(exitCodeStr))
			if exitCode != 0 {
				output := outputBuf.String()
				return output, fmt.Errorf("exit status %d", exitCode)
			}
			return outputBuf.String(), nil
		}

		outputBuf.WriteString(line)
	}
}

type sessionInfo struct {
	sb          Terminal
	lastUsed    time.Time
	hostPath    string
	idleTimeout time.Duration
}

// HostShellManager manages multiple native shell sessions across workspaces.
// It provides persistent bash sessions with workspace-specific environment isolation.
type HostShellManager struct {
	sessions map[string]*sessionInfo
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewHostShellManager initializes a native shell session manager.
func NewHostShellManager() (*HostShellManager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	hm := &HostShellManager{
		sessions: make(map[string]*sessionInfo),
		ctx:      ctx,
		cancel:   cancel,
	}

	hm.startReaper()

	logging.Info("Host Shell Manager initialized successfully")
	return hm, nil
}

func (hm *HostShellManager) startReaper() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			select {
			case <-hm.ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				hm.reap()
			}
		}
	}()
}

func (hm *HostShellManager) reap() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	now := time.Now()
	for id, s := range hm.sessions {
		if s.idleTimeout > 0 && now.Sub(s.lastUsed) > s.idleTimeout {
			logging.Info("Reaping idle shell session", "workspace_id", id, "idle_time", now.Sub(s.lastUsed))
			_ = s.sb.Cleanup(context.Background())
			delete(hm.sessions, id)
		}
	}
}

func (hm *HostShellManager) GetOrCreate(ctx context.Context, workspaceID string, hostPath string, idleTimeout time.Duration, allowedEnvVars []string, pathExtensions []string) (Terminal, error) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if s, ok := hm.sessions[workspaceID]; ok {
		s.lastUsed = time.Now()
		s.idleTimeout = idleTimeout
		return s.sb, nil
	}

	logging.Info("Creating new host shell session", "workspace_id", workspaceID, "path", hostPath)

	// Prepare a sanitized environment
	finalEnv := prepareShellEnv(hostPath, allowedEnvVars, pathExtensions)

	ps, err := newPersistentShell(hm.ctx, hostPath, finalEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to start persistent shell for workspace %s: %w", workspaceID, err)
	}

	sb := &ShellSession{
		workspaceID: workspaceID,
		hostPath:    hostPath,
		shell:       ps,
	}

	hm.sessions[workspaceID] = &sessionInfo{
		sb:          sb,
		lastUsed:    time.Now(),
		hostPath:    hostPath,
		idleTimeout: idleTimeout,
	}

	return sb, nil
}

func (hm *HostShellManager) Recycle(ctx context.Context, workspaceID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if s, ok := hm.sessions[workspaceID]; ok {
		_ = s.sb.Cleanup(ctx)
		delete(hm.sessions, workspaceID)
	}
}

func (hm *HostShellManager) ListSessions() []models.TerminalSessionView {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	out := make([]models.TerminalSessionView, 0, len(hm.sessions))
	for wid, s := range hm.sessions {
		out = append(out, models.TerminalSessionView{
			WorkspaceID: wid,
			LastUsed:    s.lastUsed,
			HostPath:    s.hostPath,
		})
	}
	return out
}

func (hm *HostShellManager) HealthCheck() (idle, active int) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	return 0, len(hm.sessions) // For now, all sessions are counted as active
}

func (hm *HostShellManager) Shutdown() {
	hm.cancel()
	hm.mu.Lock()
	defer hm.mu.Unlock()

	for _, s := range hm.sessions {
		_ = s.sb.Cleanup(context.Background())
	}
}

type ShellSession struct {
	workspaceID string
	hostPath    string
	shell       *persistentShell
}

// Execute runs a command within the managed host shell session.
func (s *ShellSession) Execute(ctx context.Context, cmd []string) (io.ReadCloser, io.ReadCloser, error) {
	rawCommand := ""
	if len(cmd) >= 3 && (cmd[0] == "sh" || cmd[0] == "bash") && cmd[1] == "-c" {
		rawCommand = cmd[2]
	} else {
		rawCommand = strings.Join(cmd, " ")
	}
	output, err := s.shell.Execute(ctx, rawCommand)

	outReader := io.NopCloser(strings.NewReader(output))
	var errReader io.ReadCloser
	if err != nil {
		errReader = io.NopCloser(strings.NewReader(err.Error()))
	} else {
		errReader = io.NopCloser(strings.NewReader(""))
	}

	return outReader, errReader, nil
}

func (s *ShellSession) Cleanup(ctx context.Context) error {
	if s.shell == nil {
		return nil
	}
	_ = s.shell.stdin.Close()
	select {
	case <-s.shell.done:
	case <-ctx.Done():
		_ = s.shell.cmd.Process.Kill()
	}
	return nil
}

var workspaceEnvTemplates = []string{
	"HOME=%s/.sandbox",
	"GOPATH=%s/.sandbox/go",
	"GOMODCACHE=%s/.sandbox/go-cache",
	"TMPDIR=%s/.sandbox/tmp",
	"TMP=%s/.sandbox/tmp",
	"TEMP=%s/.sandbox/tmp",
	"XDG_CACHE_HOME=%s/.sandbox/cache",
}

// prepareShellEnv filters the host environment and applies workspace-specific overrides.
func prepareShellEnv(hostPath string, allowedEnvVars []string, pathExtensions []string) []string {
	rawHostEnv := os.Environ()
	var hostEnv []string

	if len(allowedEnvVars) > 0 {
		allowedMap := make(map[string]bool)
		for _, v := range allowedEnvVars {
			allowedMap[v] = true
		}

		for _, e := range rawHostEnv {
			if kv := strings.SplitN(e, "=", 2); len(kv) == 2 {
				if allowedMap[kv[0]] {
					hostEnv = append(hostEnv, e)
				}
			}
		}
	} else {
		hostEnv = rawHostEnv
	}

	workspaceEnv := make([]string, len(workspaceEnvTemplates))
	for i, tpl := range workspaceEnvTemplates {
		workspaceEnv[i] = fmt.Sprintf(tpl, hostPath)
	}

	// Always construct a robust PATH to ensure host tools are available
	var currentPath string
	for _, e := range hostEnv {
		if strings.HasPrefix(e, "PATH=") {
			currentPath = strings.TrimPrefix(e, "PATH=")
			break
		}
	}
	if currentPath == "" {
		currentPath = os.Getenv("PATH")
	}

	var pathItems []string
	// 1. Extensions first
	for _, ext := range pathExtensions {
		if filepath.IsAbs(ext) {
			pathItems = append(pathItems, ext)
		} else {
			pathItems = append(pathItems, filepath.Join(hostPath, ext))
		}
	}
	// 2. Host path
	if currentPath != "" {
		pathItems = append(pathItems, filepath.SplitList(currentPath)...)
	}
	// 3. System defaults
	pathItems = append(pathItems, defaultSystemPaths...)

	seen := make(map[string]bool)
	var finalPath []string
	for _, p := range pathItems {
		if p == "" || seen[p] {
			continue
		}
		// Only add if it exists and is a directory
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			finalPath = append(finalPath, p)
			seen[p] = true
		}
	}

	pathStr := strings.Join(finalPath, string(os.PathListSeparator))
	workspaceEnv = append(workspaceEnv, "PATH="+pathStr)

	return mergeEnv(hostEnv, workspaceEnv)
}

func mergeEnv(host, overrides []string) []string {
	envMap := make(map[string]string)
	for _, e := range host {
		if kv := strings.SplitN(e, "=", 2); len(kv) == 2 {
			envMap[kv[0]] = kv[1]
		}
	}
	for _, e := range overrides {
		if kv := strings.SplitN(e, "=", 2); len(kv) == 2 {
			envMap[kv[0]] = kv[1]
		}
	}

	res := make([]string, 0, len(envMap))
	for k, v := range envMap {
		res = append(res, k+"="+v)
	}
	return res
}
