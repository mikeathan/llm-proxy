package llm

import (
	"context"
	"fmt"
	"log"
	"syscall"
	"time"

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

func (m *LLMRuntimeManager) startModelLocked(ctx context.Context, cfg models.ModelConfig) error {
	binary := m.registrar.ResolveBinary()
	modelDir := ""
	if local, ok := m.registrar.ListConfigs()["local"]; ok {
		modelDir = local.ModelDir
	}

	p := providers.NewLocalProvider(cfg, binary, modelDir, m.ModelHost())
	if err := p.StartModel(ctx); err != nil {
		return err
	}

	m.activeModel = p.ActiveModel()
	return nil
}

// signalStopLocked triggers a SIGTERM for the active model and returns a waiter.
func (m *LLMRuntimeManager) signalStopLocked() func() {
	if m.activeModel == nil {
		m.activeProvider = nil
		m.activeCloudConfig = nil
		return nil
	}

	model := m.activeModel
	cmd := model.Cmd
	name := model.Cfg.Name
	logging.Info("Signaling stop to local model", "model", name)

	if model.Cancel != nil {
		model.Cancel()
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
		// Wait on the exit-watch channel (the sole owner of cmd.Wait) rather
		// than calling cmd.Wait a second time here.
		done := model.Done()
		if done == nil {
			return
		}
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
				// 0. The process has exited (crash on launch — bad args, missing
				// model, etc.). Surface the failure immediately and clear it
				// instead of waiting out the full 5-minute startup timeout.
				if m.activeModel.Exited() {
					name := m.activeModel.Cfg.Name
					err := m.clearCrashedModelLocked(name)
					logging.Warn("Local model process exited unexpectedly; cleared",
						"model", name, "error", err)
					m.mu.Unlock()
					continue
				}

				ready := portReady(m.activeModel.Cfg.Port)

				// 1. If not ready, enforce a startup timeout to prevent zombie processes.
				if !ready {
					if time.Since(m.activeModel.Started) > startupTimeout {
						err := fmt.Errorf("failed to become ready within %v", startupTimeout)
						m.recordModelErrorLocked(m.activeModel.Cfg.Name, err)
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

// recordModelErrorLocked stores the last local-model startup/runtime failure.
// Must be called with m.mu held.
func (m *LLMRuntimeManager) recordModelErrorLocked(model string, err error) {
	if err == nil {
		m.lastModelError = fmt.Sprintf("%s: process exited", model)
		return
	}
	m.lastModelError = fmt.Sprintf("%s: %v", model, err)
}

// clearCrashedModelLocked detects and clears an active local model whose
// process has exited unexpectedly (crash on launch), recording the failure.
// It is the single shared place for crash detection so the reaper and
// GetInstance cannot diverge. Returns nil when the named active model is alive
// or starting (nothing to clear); otherwise a descriptive error. Must be
// called with m.mu held.
func (m *LLMRuntimeManager) clearCrashedModelLocked(name string) error {
	if m.activeModel == nil || m.activeModel.Cfg.Name != name {
		return nil
	}
	if !m.activeModel.Exited() {
		return nil
	}
	exitErr := m.activeModel.Err()
	if m.activeModel.Cancel != nil {
		m.activeModel.Cancel()
	}
	m.recordModelErrorLocked(name, exitErr)
	m.activeModel = nil
	m.activeProvider = nil
	if exitErr != nil {
		return fmt.Errorf("local model failed to start: %w", exitErr)
	}
	return fmt.Errorf("local model process exited unexpectedly")
}

// LastModelError returns the most recent local-model failure message, or "".
func (m *LLMRuntimeManager) LastModelError() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastModelError
}
