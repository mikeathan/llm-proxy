package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"llm-proxy/internal/app"
	"llm-proxy/internal/boot"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/platform/sandbox"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	// Sandbox-runner self-exec path (plan Phase 2b): when this process was
	// spawned as a sandboxed child it applies the OS confinement (Landlock on
	// Linux) and execs the real command — it must run before ANY other logic.
	if sandbox.RunChildIfRequested() {
		return
	}

	// `llm-proxy auth …` is the operator's command line for sign-in (setup code, reset): it never starts the server.
	if len(os.Args) > 1 && os.Args[1] == "auth" {
		os.Exit(boot.RunAuthCLI(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}

	opts := boot.ParseFlags()

	if opts.Version {
		boot.PrintVersion(&buildinfo.Info{Version: Version, Commit: Commit, BuildDate: BuildDate})
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	booted, err := boot.Startup(ctx, opts, &buildinfo.Info{Version: Version, Commit: Commit, BuildDate: BuildDate})
	if err != nil {
		os.Exit(1)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		booted.DataManager.Close(closeCtx)
	}()

	proxyApp := app.New(ctx, booted.DataManager, booted.Logger, booted.BuildInfo, opts.Record, opts.EnableRuns)

	boot.LogStartup(booted.BuildInfo, booted.BindAddr)

	if opts.Record {
		boot.LogStartupRecording()
	}

	boot.Serve(ctx, proxyApp, booted.BindAddr, booted.DataManager.RootDir())
}
