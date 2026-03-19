package llm

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"llm-proxy/internal/logging"
	"llm-proxy/internal/system_metrics"
	"llm-proxy/internal/testhooks"
	"llm-proxy/models"
)

const logBufferSize = 64 * 1024

type runningModel struct {
	cfg        models.ModelConfig
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	started    time.Time
	lastUsed   time.Time
	logs       *logging.BufferLogger
	throughput *system_metrics.TokenTracker
}

func (rm *runningModel) LastUsed() time.Time {
	return rm.lastUsed
}

func (rm *runningModel) Started() time.Time {
	return rm.started
}

func (rm *runningModel) Cfg() models.ModelConfig {
	return rm.cfg
}

func resolveModelFile(baseDir string, m models.ModelConfig) string {
	if m.Path != "" && filepath.IsAbs(m.Path) {
		return m.Path
	}
	fname := m.Filename
	if fname == "" && m.Path != "" {
		fname = filepath.Base(m.Path)
	}
	if fname == "" {
		return ""
	}
	if filepath.IsAbs(fname) {
		return fname
	}
	if baseDir != "" {
		return filepath.Join(baseDir, fname)
	}
	return fname
}

func configModelFromConfig(cfg *models.Config, model models.ModelConfig) models.ModelConfig {
	var args []string
	if len(model.Args) == 0 {
		args = append([]string(nil), cfg.Server.DefaultArgs...)
	} else {
		args = append([]string(nil), model.Args...)
	}
	
	env := make(map[string]string)
	for k, v := range cfg.Server.Environment {
		env[k] = v
	}
	for k, v := range model.Environment {
		env[k] = v
	}

	return models.ModelConfig{
		Name:        model.Name,
		Filename:    model.Filename,
		Path:        resolveModelFile(cfg.ModelDir, model),
		Args:        args,
		Port:        model.Port,
		Environment: env,
	}
}

func normalizeModelConfig(baseDir string, cfg models.ModelConfig) models.ModelConfig {
	out := cfg
	if out.Path == "" {
		out.Path = resolveModelFile(baseDir, out)
	}
	if out.Filename == "" && out.Path != "" {
		out.Filename = filepath.Base(out.Path)
	}
	return out
}

func (m *LLMRuntimeManager) syncPortWithActiveLocked(cfg models.ModelConfig) models.ModelConfig {
	if m.activeModel != nil && m.activeModel.cfg.Name == cfg.Name && cfg.Port == 0 {
		cfg.Port = m.activeModel.cfg.Port
		m.models[cfg.Name] = cfg
	}
	return cfg
}

func (m *LLMRuntimeManager) readyInstanceLocked(name string, cfg models.ModelConfig) (ModelInstance, bool) {
	if m.activeModel == nil || m.activeModel.cfg.Name != name {
		return ModelInstance{}, false
	}
	if !testhooks.PortReady(cfg.Port) {
		return ModelInstance{}, false
	}

	m.activeModel.lastUsed = time.Now()
	return modelInstance(cfg, m.modelHost), true
}

func (m *LLMRuntimeManager) activePortLocked() int {
	if m.activeModel == nil {
		return 0
	}
	return m.activeModel.cfg.Port
}

func modelInstance(cfg models.ModelConfig, host string) ModelInstance {
	return ModelInstance{
		Name: cfg.Name,
		Host: host,
		Port: cfg.Port,
		Path: cfg.Path,
		Args: cfg.Args,
	}
}

func (m *LLMRuntimeManager) startModelLocked(ctx context.Context, cfg models.ModelConfig) error {
	logBuf := logging.NewBufferLogger(logBufferSize)
	tokens := system_metrics.NewTokenTracker()
	// Create a new context derived from background for the process,
	// but we could use the passed ctx for the *startup wait* if we had one.
	// However, the process should outlive the request.
	// The passed ctx is from EnsureModel (request scoped).
	// So we should NOT use passed ctx for the command itself.
	// We continue to use context.WithCancel(context.Background()).
	// But we might want to respect passed ctx for cancellation of STARTUP?
	// For now, let's just match the signature.
	procCtx, cancel := context.WithCancel(context.Background())
	cmd := testhooks.ExecCommandContext(procCtx, m.llamaBinary, buildLaunchArgs(cfg)...)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.Stdout = io.MultiWriter(logBuf, os.Stdout, tokens)
	cmd.Stderr = io.MultiWriter(logBuf, os.Stdout, tokens)
	if len(cfg.Environment) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	logBuf.Info(fmt.Sprintf("Launching llama-server: env %v, args %v", cmd.Env, cmd.Args))
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("model start failed: %w", err)
	}

	m.activeModel = &runningModel{
		cfg:        cfg,
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
