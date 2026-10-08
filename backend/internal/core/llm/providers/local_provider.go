package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/network"
	"llm-proxy/internal/platform/process"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

const (
	logBufferSize = 10000
)

type LocalProvider struct {
	cfg         models.ModelConfig
	llamaBinary string
	modelDir    string
	host        string
	activeModel *RunningModel
}

func (p *LocalProvider) ActiveModel() *RunningModel {
	return p.activeModel
}

func NewLocalProvider(cfg models.ModelConfig, llamaBinary string, modelDir string, host string) *LocalProvider {
	return &LocalProvider{
		cfg:         cfg,
		llamaBinary: llamaBinary,
		modelDir:    modelDir,
		host:        host,
	}
}

func (p *LocalProvider) ListModels(ctx context.Context) ([]models.ProviderModelInfo, error) {
	if p.modelDir == "" {
		return nil, nil
	}

	files, err := os.ReadDir(p.modelDir)
	if err != nil {
		return nil, err
	}

	var out []models.ProviderModelInfo
	for _, f := range files {
		if !f.IsDir() && (strings.HasSuffix(f.Name(), ".gguf") || strings.HasSuffix(f.Name(), ".bin")) {
			out = append(out, models.ProviderModelInfo{ID: f.Name()})
		}
	}
	return out, nil
}

func (p *LocalProvider) TestConnection(ctx context.Context) error {
	if _, err := os.Stat(p.llamaBinary); err != nil {
		return fmt.Errorf("llama server binary not found: %w", err)
	}
	if p.modelDir != "" {
		if _, err := os.Stat(p.modelDir); err != nil {
			return fmt.Errorf("model directory not found: %w", err)
		}
	}
	return nil
}

func (p *LocalProvider) GetEndpoint(ctx context.Context) (string, http.Header, error) {
	if p.activeModel == nil {
		return "", nil, fmt.Errorf("model not running")
	}
	return network.FormatLocalURL(p.host, p.cfg.Port), nil, nil
}

// ProbeNativeTools asks the locally-launched llama-server whether the model
// can emit native OpenAI tool calls. A manager-launched llama.cpp is the same
// OpenAI-compatible wire the OpenAI-style registrations use — without this
// probe, provider-local models could never be auto-detected as native and
// always fell back to XML text mode (where native-tool models mangle the
// <tool_call> format). Shares the probe ladder with OpenAICompatibleProvider.
// Returns (false, nil) for a healthy endpoint without native tool support;
// (false, err) for transport/upstream errors so the caller falls back to XML
// without caching.
func (p *LocalProvider) ProbeNativeTools(ctx context.Context, modelID string) (bool, error) {
	if p.cfg.Port <= 0 {
		return false, fmt.Errorf("model %q not configured with a port", modelID)
	}
	endpoint := network.FormatLocalURL(p.host, p.cfg.Port) + "/v1/chat/completions"
	doer := &http.Client{Transport: network.LLMChatTransport}

	// Attempt 1: reasonable budget and deadline — the fast path.
	supported, lengthLimited, err := probeNativeToolsOnce(ctx, endpoint, nil, doer, modelID, probeNativeToolBudget, ProbeNativeToolTimeout)
	if err != nil {
		// A deadline-exceeded first attempt means the server was alive but
		// slow (still generating when we gave up) — escalate once with the
		// generous budget/deadline. Any other transport error is terminal.
		if errors.Is(err, context.DeadlineExceeded) {
			supported, _, err = probeNativeToolsOnce(ctx, endpoint, nil, doer, modelID, probeNativeToolMaxBudget, ProbeNativeToolMaxTimeout)
			return supported, err
		}
		return false, err
	}
	if supported || !lengthLimited {
		return supported, nil
	}
	// Attempt 1 was truncated by the token budget before a tool call — the
	// model may think longer before acting. Retry with the generous budget.
	supported, _, err = probeNativeToolsOnce(ctx, endpoint, nil, doer, modelID, probeNativeToolMaxBudget, ProbeNativeToolMaxTimeout)
	return supported, err
}

func (p *LocalProvider) EnsureReady(ctx context.Context) error {
	if p.activeModel != nil {
		if utils.PortReady(p.cfg.Port) {
			p.activeModel.LastUsed = time.Now()
			return nil
		}
		// Model is still warming up, do not start another one!
		return models.ErrModelStarting
	}

	if err := p.StartModel(ctx); err != nil {
		return err
	}
	return models.ErrModelStarting
}

// ErrPortInUse means the model port is held by a process this provider must not
// kill (or could not reclaim); the start fails instead of guessing.
var ErrPortInUse = errors.New("model port is in use")

const (
	portReleaseTimeout = 2 * time.Second
	portReleasePoll    = 50 * time.Millisecond
)

// portReclaimer frees a model port held by an orphan of our own server binary
// (e.g. left by a crashed proxy). Dependencies are fields so the ownership rule
// is testable without real processes.
type portReclaimer struct {
	inUse func(port int) bool
	list  func(binaryName string, activePID int) ([]process.Info, error)
	kill  func(pid int) error
}

func newPortReclaimer() portReclaimer {
	return portReclaimer{inUse: utils.PortReady, list: process.ListByBinary, kill: process.Kill}
}

// reclaim makes port available. A free port is a no-op. A busy port is freed
// only by terminating processes of the configured server binary that were
// launched with --port <port>; any other holder fails with ErrPortInUse and is
// left untouched — a wrong kill of someone else's process is worse than a
// failed start.
func (r portReclaimer) reclaim(port int, binary string) error {
	if !r.inUse(port) {
		return nil
	}
	orphans, err := r.list(filepath.Base(binary), 0)
	if err != nil {
		return fmt.Errorf("%w: port %d (cannot verify the holder: %v)", ErrPortInUse, port, err)
	}
	var owned []int
	for _, o := range orphans {
		if o.Port == port {
			owned = append(owned, o.PID)
		}
	}
	if len(owned) == 0 {
		return fmt.Errorf("%w: port %d is held by a process that is not a %s started on it; stop that process or change the model port",
			ErrPortInUse, port, filepath.Base(binary))
	}
	logging.Warn("Port held by an orphaned model server, terminating it", "port", port, "pids", owned)
	for _, pid := range owned {
		if err := r.kill(pid); err != nil {
			return fmt.Errorf("%w: port %d: terminating orphan pid %d: %v", ErrPortInUse, port, pid, err)
		}
	}
	for deadline := time.Now().Add(portReleaseTimeout); r.inUse(port); time.Sleep(portReleasePoll) {
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: port %d is still bound after terminating the orphan", ErrPortInUse, port)
		}
	}
	return nil
}

func (p *LocalProvider) StartModel(ctx context.Context) error {
	logBuf := logging.NewBufferLogger(logBufferSize)
	tokens := metrics.NewTokenTracker()
	procCtx, cancel := context.WithCancel(context.Background())

	// Pre-flight check: ensure the model path is valid before attempting launch
	if err := ValidateModelPath(p.cfg.Path); err != nil {
		cancel()
		return err
	}

	args := BuildLaunchArgs(p.cfg, p.host)
	logging.Info("Starting local model (discovery)",
		"model", p.cfg.Name,
		"binary", p.llamaBinary,
		"args", args,
		"env", p.cfg.Environment)

	if err := newPortReclaimer().reclaim(p.cfg.Port, p.llamaBinary); err != nil {
		cancel()
		logging.Error("Cannot start local model", "model", p.cfg.Name, "error", err)
		return err
	}

	cmd := utils.ExecCommandContext(procCtx, p.llamaBinary, args...)
	if runtime.GOOS != "windows" {
		attr := &syscall.SysProcAttr{Setpgid: true}
		setPdeathsig(attr)
		cmd.SysProcAttr = attr
	}
	cmd.Stdout = io.MultiWriter(logBuf, os.Stdout, tokens)
	cmd.Stderr = io.MultiWriter(logBuf, os.Stdout, tokens)

	if len(p.cfg.Environment) > 0 {
		cmd.Env = os.Environ()
		for k, v := range p.cfg.Environment {
			logging.Debug("Injecting env var", "model", p.cfg.Name, "key", k)
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	if err := cmd.Start(); err != nil {
		cancel()
		logging.Error("Failed to start local model", "model", p.cfg.Name, "error", err)
		return fmt.Errorf("model start failed: %w", err)
	}

	p.activeModel = &RunningModel{
		Cfg:        p.cfg,
		Cmd:        cmd,
		Cancel:     cancel,
		Started:    time.Now(),
		LastUsed:   time.Now(),
		Logs:       logBuf,
		Throughput: tokens,
	}
	// Watch for an unexpected exit (crash on launch). This goroutine is now the
	// single owner of cmd.Wait; Shutdown/signalStopLocked wait on Done instead.
	p.activeModel.StartWatch()

	return nil
}
