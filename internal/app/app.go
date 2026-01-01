package app

import (
	"llm-proxy/models"
	"net/http"
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

func New(cfg *models.Config, version string, commit string, buildDate string) *App {

	container := bootstrap(cfg)
	router := buildHTTP(container.BuildAppServices(), version, commit, buildDate)

	return &App{
		server: &http.Server{
			Addr:    cfg.Server.Bind,
			Handler: router,
		},
	}
}
