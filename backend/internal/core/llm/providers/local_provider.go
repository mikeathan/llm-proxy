package providers

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

func freePort(port int) {
	if !utils.PortReady(port) {
		return
	}
	logging.Warn("Port already in use, freeing before start", "port", port)
	if runtime.GOOS == "linux" {
		_ = exec.Command("fuser", "-k",
			fmt.Sprintf("%d/tcp", port)).Run()
	} else {
		_ = exec.Command("sh", "-c",
			fmt.Sprintf("lsof -ti :%d | xargs kill -9", port)).Run()
	}
	time.Sleep(300 * time.Millisecond)
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

	freePort(p.cfg.Port)

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
