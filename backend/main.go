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
	"llm-proxy/internal/platform/config"
	"llm-proxy/internal/platform/logging"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	configFlag := flag.String("config", "", "path to config file")
	flag.Parse()

	buildInfo := buildInfo()

	if *versionFlag {
		printVersion(buildInfo)
		return
	}

	logger := initLogger()
	logging.SetGlobalLogger(logger)

	configPath := *configFlag
	if configPath == "" {
		if _, err := os.Stat("backend/config/config.json"); err == nil {
			configPath = "backend/config/config.json"
		} else {
			configPath = "config/config.json"
		}
	}

	// Load configuration using ConfigManager
	cfgMgr := config.NewConfigManager(configPath)
	if err := cfgMgr.Load(); err != nil {
		logging.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	proxyApp := app.New(cfgMgr, logger, buildInfo)

	cfg := cfgMgr.GetConfig()
	logStartup(logger, buildInfo, cfg.Server.Bind)

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
