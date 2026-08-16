package app

import (
	"context"
	"net"
	"net/http"
	"time"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/network"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

type App struct {
	server       *http.Server
	services     *AppServices
	dispatcher   *automation.Dispatcher
	serverCancel context.CancelFunc
}

func (a *App) Handler() http.Handler {
	return a.server.Handler
}

func (a *App) ListenAndServe() error {
	return a.server.ListenAndServe()
}

func (a *App) Shutdown(ctx context.Context) error {
	logging.Info("Shutting down application...")

	// Cancel server base context first to immediately signal all active
	// handlers (including SSE connections) so server.Shutdown can close
	// connections without waiting for the 15s timeout.
	if a.serverCancel != nil {
		a.serverCancel()
	}

	// 1. Shutdown HTTP server
	if err := a.server.Shutdown(ctx); err != nil {
		logging.Error("HTTP server shutdown error", "error", err)
	}

	// 2. Stop dispatcher (bounded by ctx so in-flight cron jobs cannot stall
	//    shutdown past the deadline)
	if a.dispatcher != nil {
		logging.Info("Stopping automation dispatcher...")
		a.dispatcher.Stop(ctx)
	}

	// 3. Cleanup services (kills local models, shell sessions; bounded by ctx)
	a.services.Shutdown(ctx)

	return nil
}

// InitializeData prepares the data manager by loading stores and starting the
// watcher, tethered to ctx so the watcher goroutine has an explicit
// termination path (Constitution II.2 / II.14).
func InitializeData(ctx context.Context, dataMgr *storage.DataManager) error {
	// 1. Load all data (3-tier)
	if err := dataMgr.LoadAll(); err != nil {
		logging.Warn("could not load existing data stores (expected on first run)", "error", err)
	}

	// 2. Start auto-reload watcher for config files
	if err := dataMgr.Watch(ctx); err != nil {
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

func New(ctx context.Context, dataMgr *storage.DataManager, logger logging.Logger, buildInfo *buildinfo.Info, recordEnabled bool, enableRuns bool) *App {
	container := bootstrap(dataMgr, logger, recordEnabled, enableRuns)
	svc := container.BuildAppServices()

	// Tether the watcher restarted after factory reset to the app lifecycle
	// (Constitution II.2/II.14) instead of an untethered context.
	svc.AppCtx.SetRootContext(ctx)

	// Build new dispatcher with AssistantService for LLM execution
	disp, err := container.BuildDispatcher(svc)
	if err != nil {
		logging.Error("Failed to build dispatcher", "error", err)
	} else {
		container.Dispatcher = disp
		svc.SetDispatcher(disp)
		// Start dispatcher in background, tethered to app context
		go disp.Start(ctx)
	}


	router := buildHTTP(svc, container.Dispatcher, buildInfo)
	bindAddr := ResolveBindAddr(dataMgr)

	serverCtx, serverCancel := context.WithCancel(context.Background())

	return &App{
		server: &http.Server{
			Addr:              bindAddr,
			Handler:           router,
			BaseContext:       func(_ net.Listener) context.Context { return serverCtx },
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Minute,
			IdleTimeout:       120 * time.Second,
		},
		services:     svc,
		dispatcher:   container.Dispatcher,
		serverCancel: serverCancel,
	}
}
