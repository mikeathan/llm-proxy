package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"llm-proxy/internal/app"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/platform/env"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/storage"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	configFlag := flag.String("config", "", "path to config file (legacy, will be moved to data/)")
	dataFlag := flag.String("data", "data", "path to data directory containing config, secrets, and registry")
	flag.Parse()

	// Load Environment (.env files)
	env.LoadEnv()

	buildInfo := buildInfo()

	if *versionFlag {
		printVersion(buildInfo)
		return
	}

	logger := initLogger()
	logging.SetGlobalLogger(logger)

	// Initialize new Storage/Data Manager
	dataMgr, err := storage.NewDataManager(*dataFlag)
	if err != nil {
		logging.Error("failed to initialize data manager", "error", err)
		os.Exit(1)
	}

	// For Phase 1, we still might need the legacy config path if it exists outside data/
	configPath := *configFlag
	if configPath == "" {
		configPath = filepath.Join(dataMgr.RootDir(), "config.json")
	}

	// Load all data (3-tier)
	if err := dataMgr.LoadAll(); err != nil {
		logging.Warn("could not load existing data stores (expected on first run)", "error", err)
	}

	// Bootstrap using the new DataManager
	proxyApp := app.New(dataMgr, logger, buildInfo)

	// Get system bind from the new storage
	sys := dataMgr.System().Get()
	bindAddr := sys.Server.Bind
	if bindAddr == "" {
		bindAddr = "0.0.0.0:4001" // Safe default
	}

	logStartup(logger, buildInfo, bindAddr)

	// Setup Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		if err := proxyApp.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Error("server exited", "error", err)
			os.Exit(1)
		}
	}()

	<-stop
	logging.Info("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := proxyApp.Shutdown(shutdownCtx); err != nil {
		logging.Error("shutdown error", "error", err)
	}

	logging.Info("Exit complete")
}

func buildInfo() *buildinfo.Info {
	return &buildinfo.Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}

func printVersion(info *buildinfo.Info) {
	fmt.Printf(
		"llm-proxy %s (commit %s, built %s)\n",
		info.Version,
		info.Commit,
		info.BuildDate,
	)
}

func logStartup(logger logging.Logger, info *buildinfo.Info, bind string) {
	//print version info
	logging.Info(
		"LLM proxy version",
		"version", info.Version,
		"commit", info.Commit,
		"build_date", info.BuildDate,
	)

	// print bind address
	logging.Info("LLM proxy listening", "bind", bind)
}

func initLogger() logging.Logger {
	logger, err := logging.NewFileLogger(logging.Options{
		Stdout: true,
		File:   "logs/llm-proxy.log",
		Level:  logging.LevelDebug,
	})
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	return logger
}
