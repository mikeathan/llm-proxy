package app

import (
	"net/http"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/logging"
	"llm-proxy/models"
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

func New(cfg *models.Config, logger logging.Logger, buildInfo *buildinfo.Info) *App {

	container := bootstrap(cfg, logger)
	router := buildHTTP(container.BuildAppServices(), buildInfo)

	return &App{
		server: &http.Server{
			Addr:    cfg.Server.Bind,
			Handler: router,
		},
	}
}
