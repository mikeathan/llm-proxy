package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/network"
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

func (p *LocalProvider) Generate(ctx context.Context, req models.ChatRequest) (*models.ChatResponse, error) {
	return nil, fmt.Errorf("local provider Chat endpoint is not yet implemented natively; use standard model host proxying")
}

func (p *LocalProvider) GetStatus() models.ProviderStatus {
	if p.activeModel != nil {
		if utils.PortReady(p.cfg.Port) {
			return models.ProviderStatusReady
		}
		logging.Debug("Local model port not yet ready", "model", p.cfg.Name, "port", p.cfg.Port)
		return models.ProviderStatusRunning
	}
	return models.ProviderStatusReady
}

func (p *LocalProvider) GetEndpoint(ctx context.Context) (string, http.Header, error) {
	if p.activeModel == nil {
		return "", nil, fmt.Errorf("model not running")
	}
	return network.FormatLocalURL(p.host, p.cfg.Port), nil, nil
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

func (p *LocalProvider) Shutdown() error {
	if p.activeModel == nil {
		return nil
	}

	cmd := p.activeModel.Cmd
	if p.activeModel.Cancel != nil {
		p.activeModel.Cancel()
	}

	if cmd.Process != nil {
		if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil && pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}

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

	cmd := utils.ExecCommandContext(procCtx, p.llamaBinary, args...)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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

	return nil
}
