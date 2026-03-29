package app

import (
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
	router := buildHTTP(container.BuildAppServices(), buildInfo)

	// Get initial config for binding
	cfg := cfgMgr.GetConfig()

	return &App{
		server: &http.Server{
			Addr:    cfg.Server.Bind,
			Handler: router,
		},
	}
}
