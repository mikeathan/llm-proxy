package app

import (
	"context"
	"net/http"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/platform/config"
	"llm-proxy/internal/platform/logging"
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

	// Build new dispatcher with AssistantService for LLM execution
	disp, err := container.BuildDispatcher(svc)
	if err != nil {
		logging.Error("Failed to build dispatcher", "error", err)
	} else {
		container.Dispatcher = disp
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
	}
}
