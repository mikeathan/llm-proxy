package boot

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"llm-proxy/internal/buildinfo"

	"llm-proxy/internal/app"
	"llm-proxy/internal/platform/env"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/paths"
	"llm-proxy/internal/platform/storage"
)

// Flags holds the parsed command-line options for the server process.
type Flags struct {
	Version    bool
	Record     bool
	EnableRuns bool
	Data       string
}

// ParseFlags parses os.Args into Flags. The --data flag is only honored when
// explicitly supplied; an omitted --data triggers XDG/home resolution.
func ParseFlags() Flags {
	versionFlag := flag.Bool("version", false, "print version and exit")
	dataFlag := flag.String("data", "", "explicit single root directory for all config, credentials, and runtime state; when omitted resolves via LLM_PROXY_HOME or ~/.config/llm-proxy")
	recordEnabled := flag.Bool("record", false, "enable recording of LLM responses for replay testing (saved under {data}/runs/)")
	enableRuns := flag.Bool("enable-runs", false, "enable per-run output (events.jsonl, final-report.md, recordings, etc.)")
	flag.Parse()

	explicitData := ""
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "data" {
			explicitData = *dataFlag
		}
	})

	return Flags{
		Version:    *versionFlag,
		Record:     *recordEnabled,
		EnableRuns: *enableRuns,
		Data:       explicitData,
	}
}

// PrintVersion writes the build identity to stdout.
func PrintVersion(info *buildinfo.Info) {
	fmt.Printf(
		"llm-proxy %s (commit %s, built %s)\n",
		info.Version,
		info.Commit,
		info.BuildDate,
	)
}

// Booted bundles the resources assembled during Startup for the rest of main.
type Booted struct {
	DataManager *storage.DataManager
	Logger      logging.Logger
	BuildInfo   *buildinfo.Info
	BindAddr    string
}

// Startup resolves paths, seeds defaults, enforces the permission policy,
// configures logging, and initializes the data stack. It returns a wrapped error
// on any fatal step; the caller owns process exit.
func Startup(ctx context.Context, opts Flags, info *buildinfo.Info) (*Booted, error) {
	// A stderr-only fallback logger is required for the window before paths are
	// resolved and the file logger can be created at Paths.LogsDir().
	fallback := logging.NewStderrLogger(logging.LevelInfo)
	logging.SetGlobalLogger(fallback)

	// Load Environment (.env files)
	env.LoadEnv()

	p, err := paths.Resolve(opts.Data)
	if err != nil {
		fallback.Error("failed to resolve config/data directories", "error", err)
		return nil, fmt.Errorf("resolve paths: %w", err)
	}
	if err := p.SeedDefaults(); err != nil {
		fallback.Error("failed to seed default configuration", "error", err)
		return nil, fmt.Errorf("seed defaults: %w", err)
	}

	// Startup permission policy: sensitive files must not be group/world-readable.
	if err := p.EnforcePermissions(); err != nil {
		fallback.Error("config/data permission policy violation", "error", err)
		return nil, fmt.Errorf("enforce permissions: %w", err)
	}

	// Now that DataDir exists, (re)create the file logger at Paths.LogsDir().
	logger, err := logging.NewFileLogger(logging.Options{
		Stdout: true,
		File:   filepath.Join(p.LogsDir(), "llm-proxy.log"),
		Level:  logging.LevelInfo,
	})
	if err != nil {
		fallback.Error("failed to create logger", "error", err)
		return nil, fmt.Errorf("create logger: %w", err)
	}
	logging.SetGlobalLogger(logger)

	dataMgr, err := storage.NewDataManager(p)
	if err != nil {
		logging.Error("failed to initialize data manager", "error", err)
		return nil, fmt.Errorf("init data manager: %w", err)
	}

	logging.Info("data layout",
		"root", dataMgr.RootDir(),
		"meta", p.MetadataDir(),
		"runs", p.RunsDir(),
		"logs", p.LogsDir(),
	)

	if err := app.InitializeData(ctx, dataMgr); err != nil {
		return nil, fmt.Errorf("initialize data: %w", err)
	}

	bindAddr := app.ResolveBindAddr(dataMgr)
	if err := CheckPortAvailable(bindAddr); err != nil {
		logging.Error("startup failed: bind address is already in use", "address", bindAddr, "error", err)
		logging.Error("a stale llm-proxy process is likely still running; kill it with: lsof -ti :4001 | xargs kill -9")
		return nil, fmt.Errorf("check port: %w", err)
	}

	return &Booted{
		DataManager: dataMgr,
		Logger:      logger,
		BuildInfo:   info,
		BindAddr:    bindAddr,
	}, nil
}

// LogStartup emits version and bind-address lines.
func LogStartup(info *buildinfo.Info, bind string) {
	logging.Info(
		"LLM proxy version",
		"version", info.Version,
		"commit", info.Commit,
		"build_date", info.BuildDate,
	)

	logging.Info("LLM proxy listening", "bind", bind)
}

// LogStartupRecording emits the recording-enabled notice.
func LogStartupRecording() {
	logging.Info("Recording enabled — LLM responses saved under {data}/runs/")
}

// Serve launches the HTTP server in a goroutine and blocks until ctx is
// cancelled, then performs a bounded graceful shutdown.
func Serve(ctx context.Context, proxyApp *app.App, bindAddr string) {
	go func() {
		if err := proxyApp.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Error("server exited", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logging.Info("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := proxyApp.Shutdown(shutdownCtx); err != nil {
		logging.Error("shutdown error", "error", err)
	}

	logging.Info("Exit complete")
}

// CheckPortAvailable verifies that the given TCP address is not already bound.
// A failure here suggests a stale process from a previous session is still
// holding the port, which would otherwise surface as a confusing EADDRINUSE
// error inside ListenAndServe after the app is fully bootstrapped.
func CheckPortAvailable(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("address %s is already in use: %w", addr, err)
	}
	ln.Close()
	return nil
}
