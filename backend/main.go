package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	// Setup Graceful Shutdown Context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Configure Data Stack
	if err := app.InitializeData(dataMgr); err != nil {
		os.Exit(1)
	}
	defer dataMgr.Close()

	// Bootstrap using the new DataManager and app context
	proxyApp := app.New(ctx, dataMgr, logger, buildInfo)
	bindAddr := app.ResolveBindAddr(dataMgr)

	logStartup(logger, buildInfo, bindAddr)

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
		Level:  logging.LevelInfo,
	})
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	return logger
}
