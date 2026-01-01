package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"llm-proxy/internal/app"
	"llm-proxy/utils"
)

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

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

	// Load configuration
	cfg, err := utils.LoadConfig(configPath)
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return
	}
	// Load environment variables
	utils.LoadEnv()

	proxyApp := app.New(cfg, buildInfo.Version, buildInfo.Commit, buildInfo.BuildDate)

	logStartup(buildInfo, cfg.Server.Bind)

	log.Fatal(proxyApp.ListenAndServe())
}

func buildInfo() BuildInfo {
	return BuildInfo{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}

func printVersion(info BuildInfo) {
	fmt.Printf(
		"llm-proxy %s (commit %s, built %s)\n",
		info.Version,
		info.Commit,
		info.BuildDate,
	)
}

func logStartup(info BuildInfo, bind string) {

	// print loaded env file
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}
	log.Printf("Loaded env file .env.%s", env)

	//print version info
	log.Printf(
		"LLM proxy version %s (commit %s, built %s)",
		info.Version,
		info.Commit,
		info.BuildDate,
	)

	// print bind address
	log.Printf("LLM proxy listening on %s", bind)
}
