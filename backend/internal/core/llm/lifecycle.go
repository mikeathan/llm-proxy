package llm

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"syscall"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

func (m *LLMRuntimeManager) startModelLocked(ctx context.Context, cfg models.ModelConfig) error {
	logBuf := logging.NewBufferLogger(logBufferSize)
	tokens := metrics.NewTokenTracker()
	procCtx, cancel := context.WithCancel(context.Background())

	args := buildLaunchArgs(cfg)
	logging.Info("Starting local model (runtime)", "model", cfg.Name, "binary", m.llamaBinary, "args", args)

	cmd := utils.ExecCommandContext(procCtx, m.llamaBinary, args...)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.Stdout = io.MultiWriter(logBuf, os.Stdout, tokens)
	cmd.Stderr = io.MultiWriter(logBuf, os.Stdout, tokens)

	if len(cfg.Environment) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Environment {
			logging.Debug("Injecting env var", "model", cfg.Name, "key", k)
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	if err := cmd.Start(); err != nil {
		cancel()
		logging.Error("Failed to start local model (runtime)", "model", cfg.Name, "error", err)
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

// signalStopLocked triggers a SIGTERM for the active model and returns a waiter.
func (m *LLMRuntimeManager) signalStopLocked() func() {
	if m.activeModel == nil {
		m.activeProvider = nil
		m.activeCloudConfig = nil
		return nil
	}

	cmd := m.activeModel.cmd
	if m.activeModel.cancel != nil {
		m.activeModel.cancel()
	}

	// Try graceful stop
	if cmd.Process != nil {
		if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil && pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}

	m.activeModel = nil
	m.activeProvider = nil
	m.activeCloudConfig = nil

	return func() {
		done := make(chan struct{})
		go func() {
			cmd.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			if cmd.Process != nil {
				if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil && pgid > 0 {
					_ = syscall.Kill(-pgid, syscall.SIGKILL)
				} else {
					_ = cmd.Process.Kill()
				}
			}
		}
	}
}

// reapIdleModels periodically checks and shuts down unused local models.
func (m *LLMRuntimeManager) reapIdleModels(reapInterval time.Duration) {
	t := time.NewTicker(reapInterval)
	defer t.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-t.C:
			m.mu.Lock()

			if m.activeModel != nil {
				if time.Since(m.activeModel.lastUsed) > m.idleTimeout {
					log.Printf("Idle timeout on model %s → stopping", m.activeModel.cfg.Name)
					waiter := m.signalStopLocked()
					m.mu.Unlock()
					if waiter != nil {
						waiter()
					}
					continue
				}
			}

			m.mu.Unlock()
		}
	}
}

// ActiveLogs returns the buffered output for the current model.
func (m *LLMRuntimeManager) ActiveLogs() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeModel == nil || m.activeModel.logs == nil {
		return ""
	}
	return m.activeModel.logs.String()
}

// LastTokensPerSecond returns the throughput of the active model.
func (m *LLMRuntimeManager) LastTokensPerSecond() (float64, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeModel != nil && m.activeModel.throughput != nil {
		return m.activeModel.throughput.LastTokensPerSecond()
	}
	return 0, time.Time{}
}
