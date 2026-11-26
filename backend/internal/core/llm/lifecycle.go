package llm

import (
	"context"
	"fmt"
	"log"
	"maps"
	"slices"
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

	// A new server process will start next time — its serving context must be
	// reconciled again (the flag is otherwise cleared by Sync).
	delete(m.servingCtxSynced, name)
	delete(m.servingCtxSilentSince, name)

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

	m.retainActiveModelLogsLocked()
	m.stopping = model.Done()
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

// awaitStopLocked waits, with the lock released, until the last server told to stop has exited, so the next start never
// finds the port still held by it. It reports whether it released the lock (the caller must re-check its state). The
// wait is bounded by the stop's own waiter, which kills the server after shutdownTimeout.
func (m *LLMRuntimeManager) awaitStopLocked(ctx context.Context) (bool, error) {
	done := m.stopping
	if done == nil {
		return false, nil
	}
	select {
	case <-done:
		m.stopping = nil
		return false, nil
	default:
	}
	m.mu.Unlock()
	var err error
	select {
	case <-done:
	case <-ctx.Done():
		err = ctx.Err()
	}
	m.mu.Lock()
	return true, err
}

// clearSlotLocked frees the local slot for another model. A model still loading is left to finish (the caller is told
// to wait); a running one is stopped and waited for; a stop made elsewhere (a settings save) is waited for until that
// server has exited. retry means the lock was released and the caller must re-check its state.
func (m *LLMRuntimeManager) clearSlotLocked(ctx context.Context) (retry bool, err error) {
	if m.stillLoadingLocked() && !isExplicitStart(ctx) {
		return false, models.ErrModelStarting
	}
	if waiter := m.signalStopLocked(); waiter != nil {
		m.mu.Unlock()
		waiter()
		m.mu.Lock()
		return true, nil
	}
	return m.awaitStopLocked(ctx)
}

// loadingGrace is how long a local model that is still loading is protected from being stopped by a request for
// another model. Without it two callers wanting different models stopped each other's server before either was ready.
const loadingGrace = 2 * time.Minute

// stillLoadingLocked reports whether the active local model is still loading and within loadingGrace. llama-server opens
// its port before the model has loaded, so "loaded" is the window the server has reported (servingCtxSynced), not an
// open port.
func (m *LLMRuntimeManager) stillLoadingLocked() bool {
	a := m.activeModel
	return a != nil && !a.Exited() && !m.servingCtxSynced[a.Cfg.Name] && time.Since(a.Started) < loadingGrace
}

type explicitStartKey struct{}

// WithExplicitStart marks a request as the owner's own Start of a model (the Models page), which may stop a model that
// is still loading; requests a run or an outside caller make wait for it instead.
func WithExplicitStart(ctx context.Context) context.Context {
	return context.WithValue(ctx, explicitStartKey{}, true)
}

func isExplicitStart(ctx context.Context) bool {
	explicit, _ := ctx.Value(explicitStartKey{}).(bool)
	return explicit
}

// launchChangedLocked reports whether starting cfg would launch a different server from the running one: other
// arguments, model file or environment. Settings that are not part of the launch (temperature, budgets) do not count.
// The running side is what the server was started with (LaunchArgs), so a change of the model host made just before this
// save (the System settings handler sets it first) still counts.
func (m *LLMRuntimeManager) launchChangedLocked(cfg models.ModelConfig) bool {
	running := m.activeModel
	next := m.enrichModelLocked(cfg)
	if next.Port == 0 {
		next.Port = running.Cfg.Port
	}
	host := m.registrar.ModelHost()
	launched := running.LaunchArgs
	if launched == nil {
		launched = providers.BuildLaunchArgs(running.Cfg, host)
	}
	return !slices.Equal(launched, providers.BuildLaunchArgs(next, host)) || !maps.Equal(running.Cfg.Environment, next.Environment)
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

// ActiveLogs returns the buffered output for the current model, or the last
// stopped/crashed model's output when no model is running — so a post-mortem of
// a crash is still visible via /admin/api/logs after the model is cleared.
func (m *LLMRuntimeManager) ActiveLogs() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeModel == nil || m.activeModel.Logs == nil {
		return m.lastModelLogs
	}
	return m.activeModel.Logs.String()
}

// retainActiveModelLogsLocked snapshots the active model's buffered output into
// lastModelLogs before the model is cleared. Must be called with m.mu held.
func (m *LLMRuntimeManager) retainActiveModelLogsLocked() {
	if m.activeModel != nil && m.activeModel.Logs != nil {
		m.lastModelLogs = m.activeModel.Logs.String()
	}
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
	m.retainActiveModelLogsLocked()
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
