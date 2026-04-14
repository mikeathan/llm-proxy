package app

import (
	"context"
	"net/http"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/platform/config"
	"llm-proxy/internal/platform/logging"
)

type App struct {
	server     *http.Server
	services   *AppServices
	dispatcher *automation.Dispatcher
}

func (a *App) Handler() http.Handler {
	return a.server.Handler
}

func (a *App) ListenAndServe() error {
	return a.server.ListenAndServe()
}

func (a *App) Shutdown(ctx context.Context) error {
	logging.Info("Shutting down application...")

	// 1. Shutdown HTTP server
	if err := a.server.Shutdown(ctx); err != nil {
		logging.Error("HTTP server shutdown error", "error", err)
	}

	// 2. Stop dispatcher
	if a.dispatcher != nil {
		logging.Info("Stopping automation dispatcher...")
		a.dispatcher.Stop()
	}

	// 3. Cleanup services (kills local models)
	a.services.Shutdown()

	return nil
}

func New(cfgMgr *config.ConfigManager, logger logging.Logger, buildInfo *buildinfo.Info) *App {

	container := bootstrap(cfgMgr, logger)
	svc := container.BuildAppServices()

	// Build new dispatcher with AssistantService for LLM execution
	disp, err := container.BuildDispatcher(svc)
	if err != nil {
		logging.Error("Failed to build dispatcher", "error", err)
	} else {
		container.Dispatcher = disp
		svc.SetDispatcher(disp)
		// Start dispatcher in background
		go disp.Start(context.Background())
	}

	router := buildHTTP(svc, container.Dispatcher, buildInfo)

	// Get initial config for binding
	cfg := cfgMgr.GetConfig()

	return &App{
		server: &http.Server{
			Addr:    cfg.Server.Bind,
			Handler: router,
		},
		services:   svc,
		dispatcher: container.Dispatcher,
	}
}
