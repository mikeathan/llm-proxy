package nmap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/platform/progexec"
)

// EngineName labels results produced by this engine.
const EngineName = "nmap"

// Run limits. The scan must end inside netaudit.ScanBudget so the run can still write its report; a scan that runs out
// of time returns the hosts it finished.
const (
	scanTimeout   = netaudit.ScanBudget
	maxOutput     = 4 << 20
	runDirPattern = "netaudit-nmap-*"
	nmapDataFile  = "nmap-services"
)

// Program is how nmap is found and installed. It is the single source of the install message.
var Program = progexec.Program{
	Name:        "nmap",
	Binary:      "nmap",
	VersionArgs: []string{"--version"},
	InstallHint: map[string]string{
		"darwin": "brew install nmap",
		"linux":  "sudo apt install nmap   (or: sudo dnf install nmap)",
	},
}

// defaultDataDirs are where nmap keeps its data files (service names, probes) on the supported hosts.
var defaultDataDirs = []string{"/usr/share/nmap", "/usr/local/share/nmap", "/opt/homebrew/share/nmap"}

// Runner starts guarded programs; progexec.Runner satisfies it.
type Runner interface {
	Run(ctx context.Context, p progexec.Program, inv progexec.Invocation) (progexec.Output, error)
}

// Engine runs nmap through a guarded-program runner.
type Engine struct {
	runner   Runner
	detect   func(context.Context) (progexec.Installed, error)
	dataDirs []string
}

// New builds the engine. Detection results are not cached, so installing nmap takes effect on the next scan.
func New(runner Runner) *Engine {
	return &Engine{
		runner:   runner,
		detect:   func(ctx context.Context) (progexec.Installed, error) { return progexec.Detect(ctx, Program) },
		dataDirs: defaultDataDirs,
	}
}

// Name labels results.
func (*Engine) Name() string { return EngineName }

// Available reports why nmap cannot be used; nil means it is installed and its data files were found.
func (e *Engine) Available(ctx context.Context) error {
	if _, err := e.detect(ctx); err != nil {
		return err
	}
	if _, err := e.dataDir(); err != nil {
		return err
	}
	return nil
}

// Supports is true for every mode: nmap does them all.
func (*Engine) Supports(m netaudit.Mode) bool { return m.Valid() }

// Scan runs nmap for an authorised request and parses its XML.
func (e *Engine) Scan(ctx context.Context, req netaudit.ScanRequest) (netaudit.Result, error) {
	args, err := Args(req)
	if err != nil {
		return netaudit.Result{}, err
	}
	installed, err := e.detect(ctx)
	if err != nil {
		return netaudit.Result{}, err
	}
	dataDir, err := e.dataDir()
	if err != nil {
		return netaudit.Result{}, err
	}
	// The run directory is private to the scan and outside every agent workspace; nmap's HOME and temp files go there.
	root, err := os.MkdirTemp("", runDirPattern)
	if err != nil {
		return netaudit.Result{}, fmt.Errorf("failed to create the scan directory: %w", err)
	}
	defer os.RemoveAll(root)

	out, err := e.runner.Run(ctx, Program, progexec.Invocation{
		Args:      args,
		Env:       append(progexec.CleanEnv(root), "NMAPDIR="+dataDir),
		Root:      root,
		ReadPaths: []string{dataDir, filepath.Dir(installed.Path)},
		Timeout:   runLimit(req),
		MaxOutput: maxOutput,
	})
	if errors.Is(err, progexec.ErrTimedOut) {
		return netaudit.Result{
			Hosts: ParsePartialXML(out.Stdout),
			Warnings: []string{fmt.Sprintf("the scan hit its %s time limit and was stopped: the hosts listed finished before then, "+
				"so more may exist. Scan fewer addresses at a time or use a quick scan.", scanTimeout)},
		}, nil
	}
	if err != nil {
		return netaudit.Result{}, err
	}
	hosts, err := ParseXML(strings.NewReader(string(out.Stdout)))
	if err != nil {
		return netaudit.Result{}, err
	}
	res := netaudit.Result{Hosts: hosts}
	if out.Truncated {
		res.Warnings = append(res.Warnings, "nmap output was cut at the size limit; some hosts may be missing")
	}
	return res, nil
}

// runLimit is how long nmap may run for a request: the caller's bound when it set one (never longer than the engine's own),
// else the engine's.
func runLimit(req netaudit.ScanRequest) time.Duration {
	if req.Timeout > 0 && req.Timeout < scanTimeout {
		return req.Timeout
	}
	return scanTimeout
}

// dataDir finds the directory with nmap's data files; the scan pins nmap to it through NMAPDIR so nmap never
// searches the home directory or the current directory for replacements.
func (e *Engine) dataDir() (string, error) {
	for _, dir := range e.dataDirs {
		if _, err := os.Stat(filepath.Join(dir, nmapDataFile)); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("nmap data files (%s) were not found in %v", nmapDataFile, e.dataDirs)
}
