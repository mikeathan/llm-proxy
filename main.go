package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"llm-proxy/internal/app"
	"llm-proxy/internal/logging"
	"llm-proxy/utils"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

const configPath = "config/config.json"

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	buildInfo := buildInfo()

	if *versionFlag {
		printVersion(buildInfo)
		return
	}

	logger := initLogger()

	// Load configuration
	cfg, err := utils.LoadConfig(configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		return
	}
	// Load environment variables
	utils.LoadEnv()

	proxyApp := app.New(cfg, logger, buildInfo)

	logStartup(logger, buildInfo, cfg.Server.Bind)

	if err := proxyApp.ListenAndServe(); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func buildInfo() *app.BuildInfo {
	return &app.BuildInfo{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}

func printVersion(info *app.BuildInfo) {
	fmt.Printf(
		"llm-proxy %s (commit %s, built %s)\n",
		info.Version,
		info.Commit,
		info.BuildDate,
	)
}

func logStartup(logger logging.Logger, info *app.BuildInfo, bind string) {
	// print loaded env file
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}
	logger.Info("Loaded env file", "file", ".env."+env)

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
		Level:  logging.LevelInfo,
	})
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	return logger
}
