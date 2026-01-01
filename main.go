package main

import (
	"flag"
	"fmt"
	"log"

	"llm-proxy/internal/app"
	"llm-proxy/utils"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("llm-proxy %s (commit %s, built %s)\n", Version, Commit, BuildDate)
		return
	}

	// Load configuration
	cfg, err := utils.LoadConfig("config/config.json")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}
	// Load environment variables
	utils.LoadEnv()

	proxyApp := app.New(cfg)

	log.Printf("LLM proxy version %s (commit %s, built %s)", Version, Commit, BuildDate)
	log.Printf("LLM proxy listening on %s", cfg.Server.Bind)

	log.Fatal(proxyApp.ListenAndServe())
}
