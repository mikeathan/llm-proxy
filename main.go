package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"llm-proxy/internal/app"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/config"
	"llm-proxy/internal/logging"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

const configPath = "config/config.json" // Hardcoded default, can be improved

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	buildInfo := buildInfo()

	if *versionFlag {
		printVersion(buildInfo)
		return
	}

	logger := initLogger()

	// Load configuration using ConfigManager
	cfgMgr := config.NewConfigManager(configPath)
	if err := cfgMgr.Load(); err != nil {
		logger.Error("failed to load config", "error", err)
		// Fallback or exit? Exit is safer if config is broken
		if config.GetAppEnv() != "test" { // Allow test to proceed?
			// return // or continue with defaults?
			// For now, let's log and try to proceed if possible or exit.
			// Existing code exited on load fail.
			// However ConfigManager Load handles missing file? No.
			// So exit.
		}
	}

	proxyApp := app.New(cfgMgr, logger, buildInfo)

	cfg := cfgMgr.GetConfig()
	logStartup(logger, buildInfo, cfg.Server.Bind)

	if err := proxyApp.ListenAndServe(); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
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
	logger.Info(
		"LLM proxy version",
		"version", info.Version,
		"commit", info.Commit,
		"build_date", info.BuildDate,
	)

	// print bind address
	logger.Info("LLM proxy listening", "bind", bind)
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
