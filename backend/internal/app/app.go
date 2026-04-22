package app

import (
	"context"
	"net/http"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/network"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
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

// InitializeData prepares the data manager by loading stores and starting the watcher.
func InitializeData(dataMgr *storage.DataManager) error {
	// 1. Load all data (3-tier)
	if err := dataMgr.LoadAll(); err != nil {
		logging.Warn("could not load existing data stores (expected on first run)", "error", err)
	}

	// 2. Start auto-reload watcher for config files
	if err := dataMgr.Watch(); err != nil {
		logging.Error("failed to start config watcher", "error", err)
		return err
	}

	return nil
}

// ResolveBindAddr determines the server binding address from configuration.
func ResolveBindAddr(dataMgr *storage.DataManager) string {
	sys := dataMgr.System().Get()
	if sys.Server.Bind == "" {
		return network.JoinDefault(models.AddrAllInterfaces)
	}
	return sys.Server.Bind
}

func New(dataMgr *storage.DataManager, logger logging.Logger, buildInfo *buildinfo.Info) *App {
	container := bootstrap(dataMgr, logger)
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
	bindAddr := ResolveBindAddr(dataMgr)

	return &App{
		server: &http.Server{
			Addr:    bindAddr,
			Handler: router,
		},
		services:   svc,
		dispatcher: container.Dispatcher,
	}
}
