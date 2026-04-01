package app

import (
	"context"
	"net/http"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/config"
	"llm-proxy/internal/logging"
)

type App struct {
	server *http.Server
}

func (a *App) Handler() http.Handler {
	return a.server.Handler
}

func (a *App) ListenAndServe() error {
	return a.server.ListenAndServe()
}

func New(cfgMgr *config.ConfigManager, logger logging.Logger, buildInfo *buildinfo.Info) *App {

	container := bootstrap(cfgMgr, logger)
	svc := container.BuildAppServices()
	ws := container.BuildWorkspaceServices()

	// Build new dispatcher (Phase 1: created but not yet wired into HTTP or started)
	disp, err := container.BuildDispatcher()
	if err != nil {
		logger.Error("Failed to build dispatcher", "error", err)
	} else {
		container.Dispatcher = disp
	}

	// Start workspace scheduler in background
	if ws.Scheduler != nil {
		go ws.Scheduler.Start(context.Background())
	}

	router := buildHTTP(svc, ws, buildInfo)

	// Get initial config for binding
	cfg := cfgMgr.GetConfig()

	return &App{
		server: &http.Server{
			Addr:    cfg.Server.Bind,
			Handler: router,
		},
	}
}
