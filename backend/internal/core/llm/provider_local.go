package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

const logBufferSize = 64 * 1024

type LocalProvider struct {
	cfg         models.ModelConfig
	llamaBinary string
	modelDir    string
	activeModel *runningModel
}

type runningModel struct {
	cfg        models.ModelConfig
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	started    time.Time
	lastUsed   time.Time
	logs       *logging.BufferLogger
	throughput *metrics.TokenTracker
}

func (r *runningModel) Cfg() models.ModelConfig {
	return r.cfg
}

func (r *runningModel) LastUsed() time.Time {
	return r.lastUsed
}

func NewLocalProvider(cfg models.ModelConfig, llamaBinary string, modelDir string) *LocalProvider {
	return &LocalProvider{
		cfg:         cfg,
		llamaBinary: llamaBinary,
		modelDir:    modelDir,
	}
}

func (p *LocalProvider) ListModels(ctx context.Context) ([]string, error) {
	if p.modelDir == "" {
		return nil, nil
	}

	files, err := os.ReadDir(p.modelDir)
	if err != nil {
		return nil, err
	}

	var models []string
	for _, f := range files {
		if !f.IsDir() && (strings.HasSuffix(f.Name(), ".gguf") || strings.HasSuffix(f.Name(), ".bin")) {
			models = append(models, f.Name())
		}
	}
	return models, nil
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

func (p *LocalProvider) Generate(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("local provider Chat endpoint is not yet implemented natively; use standard model host proxying")
}

func (p *LocalProvider) GetStatus() ProviderStatus {
	if p.activeModel != nil {
		if utils.PortReady(p.cfg.Port) {
			return ProviderStatusReady
		}
		return ProviderStatusRunning
	}
	return ProviderStatusReady // Ready for use, even if not running
}

func (p *LocalProvider) GetEndpoint(ctx context.Context) (string, http.Header, error) {
	if p.activeModel == nil {
		return "", nil, fmt.Errorf("model not running")
	}
	return fmt.Sprintf("http://127.0.0.1:%d", p.cfg.Port), nil, nil
}

func (p *LocalProvider) EnsureReady(ctx context.Context) error {
	if p.activeModel != nil && utils.PortReady(p.cfg.Port) {
		p.activeModel.lastUsed = time.Now()
		return nil
	}

	if err := p.startModel(ctx); err != nil {
		return err
	}
	return ErrModelStarting
}

func (p *LocalProvider) Shutdown() error {
	if p.activeModel == nil {
		return nil
	}

	cmd := p.activeModel.cmd
	if p.activeModel.cancel != nil {
		p.activeModel.cancel()
	}

	// Try graceful stop
	if cmd.Process != nil {
		if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil && pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		if cmd.Process != nil {
			if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil && pgid > 0 {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				_ = cmd.Process.Kill()
			}
		}
	}

	p.activeModel = nil
	return nil
}

func (p *LocalProvider) startModel(ctx context.Context) error {
	logBuf := logging.NewBufferLogger(logBufferSize)
	tokens := metrics.NewTokenTracker()
	procCtx, cancel := context.WithCancel(context.Background())
	cmd := utils.ExecCommandContext(procCtx, p.llamaBinary, buildLaunchArgs(p.cfg)...)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.Stdout = io.MultiWriter(logBuf, os.Stdout, tokens)
	cmd.Stderr = io.MultiWriter(logBuf, os.Stdout, tokens)

	if len(p.cfg.Environment) > 0 {
		cmd.Env = os.Environ()
		for k, v := range p.cfg.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("model start failed: %w", err)
	}

	p.activeModel = &runningModel{
		cfg:        p.cfg,
		cmd:        cmd,
		cancel:     cancel,
		started:    time.Now(),
		lastUsed:   time.Now(),
		logs:       logBuf,
		throughput: tokens,
	}

	return nil
}

func buildLaunchArgs(cfg models.ModelConfig) []string {
	args := []string{"-m", cfg.Path, "--port", fmt.Sprint(cfg.Port)}
	return append(args, sanitizeArgs(cfg.Args)...)
}

func sanitizeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	skipNext := false
	for i := 0; i < len(args); i++ {
		if skipNext {
			skipNext = false
			continue
		}
		arg := args[i]
		if arg == "--n-batch" {
			skipNext = true
			continue
		}
		out = append(out, arg)
	}
	return out
}
