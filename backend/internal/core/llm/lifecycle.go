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

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

func (m *LLMRuntimeManager) startModelLocked(ctx context.Context, cfg models.ModelConfig) error {
	logBuf := logging.NewBufferLogger(logBufferSize)
	tokens := metrics.NewTokenTracker()
	procCtx, cancel := context.WithCancel(context.Background())

	args := providers.BuildLaunchArgs(cfg)
	binary := m.registrar.DefaultBinary()

	logging.Info("Starting local model (runtime)", 
		"model", cfg.Name, 
		"binary", binary, 
		"args", args, 
		"env", cfg.Environment)

	cmd := utils.ExecCommandContext(procCtx, binary, args...)
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

	m.activeModel = &providers.RunningModel{
		Cfg:        cfg,
		Cmd:        cmd,
		Cancel:     cancel,
		Started:    time.Now(),
		LastUsed:   time.Now(),
		Logs:       logBuf,
		Throughput: tokens,
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

	cmd := m.activeModel.Cmd
	name := m.activeModel.Cfg.Name
	logging.Info("Signaling stop to local model", "model", name)

	if m.activeModel.Cancel != nil {
		m.activeModel.Cancel()
	}

	// Try graceful stop (SIGTERM) to the process group
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
			_ = cmd.Wait()
			close(done)
		}()

		select {
		case <-done:
			logging.Info("Local model exited gracefully", "model", name)
		case <-time.After(shutdownTimeout):
			if cmd.Process != nil {
				logging.Warn("Local model graceful shutdown timed out; forcing kill", "model", name)
				if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil && pgid > 0 {
					_ = syscall.Kill(-pgid, syscall.SIGKILL)
				} else {
					_ = cmd.Process.Kill()
				}
			}
			<-done // Ensure wait is complete
		}
	}
}

// reapIdleModels periodically checks and shuts down unused local models.
func (m *LLMRuntimeManager) reapIdleModels(reapInterval time.Duration) {
	t := time.NewTicker(reapInterval)
	defer t.Stop()

	startupTimeout := 5 * time.Minute

	for {
		select {
		case <-m.stopCh:
			return
		case <-t.C:
			m.mu.Lock()

			if m.activeModel != nil {
				ready := portReady(m.activeModel.Cfg.Port)

				// 1. If not ready, enforce a startup timeout to prevent zombie processes.
				if !ready {
					if time.Since(m.activeModel.Started) > startupTimeout {
						log.Printf("Model %s failed to become ready within %v → stopping", m.activeModel.Cfg.Name, startupTimeout)
						waiter := m.signalStopLocked()
						m.mu.Unlock()
						if waiter != nil {
							waiter()
						}
						continue
					}
					m.mu.Unlock()
					continue
				}

				// 2. If ready but idleTimeout is 0, never reap.
				if m.idleTimeout <= 0 {
					m.mu.Unlock()
					continue
				}

				// 3. Check for idle timeout.
				if time.Since(m.activeModel.LastUsed) > m.idleTimeout {
					log.Printf("Idle timeout on model %s → stopping", m.activeModel.Cfg.Name)
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
	if m.activeModel == nil || m.activeModel.Logs == nil {
		return ""
	}
	return m.activeModel.Logs.String()
}

// LastTokensPerSecond returns the throughput of the active model.
func (m *LLMRuntimeManager) LastTokensPerSecond() (float64, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeModel != nil && m.activeModel.Throughput != nil {
		return m.activeModel.Throughput.LastTokensPerSecond()
	}
	return 0, time.Time{}
}
