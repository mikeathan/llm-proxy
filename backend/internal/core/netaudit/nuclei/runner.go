package nuclei

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"llm-proxy/internal/platform/progexec"
)

// Program is how nuclei is found and installed. Whether it is installed is asked with progexec.Locate and never by starting it:
// nuclei writes files into the folder it is started in, even to print its version.
var Program = progexec.Program{
	Name:        "nuclei",
	Binary:      "nuclei",
	VersionArgs: []string{"-version"},
	InstallHint: map[string]string{
		"darwin": "brew install nuclei",
		"linux":  "download the nuclei release for this computer from github.com/projectdiscovery/nuclei/releases and put the nuclei program on the PATH",
	},
}

const (
	// maxOutput caps what a run's output may be read as.
	maxOutput = 1 << 20
	// killGrace is how much longer than the program's own time limit the runner waits before it kills the process.
	killGrace = 30 * time.Second
	// maxTemplateSize is the largest template file the download accepts.
	maxTemplateSize = 256 << 10
	// rawBase is where the reviewed files are fetched from, at the reviewed tag.
	rawBase = "https://raw.githubusercontent.com/projectdiscovery/nuclei-templates/"
	runDir  = "netaudit-nuclei-*"
)

// Runner starts guarded programs; progexec.Runner satisfies it.
type Runner interface {
	Run(ctx context.Context, p progexec.Program, inv progexec.Invocation) (progexec.Output, error)
}

// Checker runs the reviewed templates against web services. The template folder is private to this program.
type Checker struct {
	runner Runner
	root   string
	list   []Template
	locate func() (string, error)
}

// NewChecker builds the checker over the template folder and the reviewed list.
func NewChecker(runner Runner, templateRoot string) *Checker {
	return &Checker{runner: runner, root: templateRoot, list: Reviewed, locate: func() (string, error) { return progexec.Locate(Program) }}
}

// Result is what a run found and whether it was cut short by its time limit.
type Result struct {
	Hits    []Hit
	Partial bool
}

// Ready says whether a run can start now, and when it cannot, why in plain words: nuclei is not installed, or the reviewed
// templates are not (all) downloaded.
func (c *Checker) Ready() (bool, string) {
	if _, err := c.locate(); err != nil {
		return false, err.Error()
	}
	if st := StatusOf(c.root, c.list); st.Present < st.Total {
		return false, fmt.Sprintf("the reviewed web-check templates are not downloaded (%d of %d present)", st.Present, st.Total)
	}
	return true, ""
}

// Check runs every reviewed template against the targets. It verifies the templates first and starts nothing if any differs.
func (c *Checker) Check(ctx context.Context, targets []Target, limit time.Duration) (Result, error) {
	paths, err := Verify(c.root, c.list)
	if err != nil {
		return Result{}, err
	}
	args, err := Args(Request{Targets: targets, Templates: paths, Timeout: limit})
	if err != nil {
		return Result{}, err
	}
	binary, err := c.locate()
	if err != nil {
		return Result{}, err
	}
	run, err := os.MkdirTemp("", runDir)
	if err != nil {
		return Result{}, fmt.Errorf("failed to create the web check directory: %w", err)
	}
	defer os.RemoveAll(run)
	out, err := c.runner.Run(ctx, Program, progexec.Invocation{
		Args: args, Env: progexec.CleanEnv(run), Root: run,
		ReadPaths: []string{c.root, filepath.Dir(binary)}, Timeout: limit + killGrace, MaxOutput: maxOutput,
	})
	res := Result{Hits: ParseHits(out.Stdout, Index(c.list), targets), Partial: out.Truncated}
	if errors.Is(err, progexec.ErrTimedOut) {
		res.Partial = true
		return res, nil
	}
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// Opener fetches a URL through the guarded network client: NetworkTools.Open satisfies it.
type Opener func(ctx context.Context, url string) (*http.Response, error)

// Status counts the reviewed templates whose file is present with the reviewed checksum.
type Status struct {
	Present int `json:"present"`
	Total   int `json:"total"`
}

// StatusOf counts what is in the folder.
func StatusOf(root string, list []Template) Status {
	st := Status{Total: len(list)}
	for _, t := range list {
		if _, err := verifyOne(root, t); err == nil {
			st.Present++
		}
	}
	return st
}

// Download fetches each reviewed template from the reviewed tag and writes it only when its checksum is the reviewed one. A file
// that is missing, too large or different is not written, and the error names its template; the others are still fetched.
func Download(ctx context.Context, open Opener, root string, list []Template) error {
	var failed []string
	for _, t := range list {
		if err := fetchOne(ctx, open, root, t); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", t.ID, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("nuclei: these templates could not be downloaded as reviewed: %s", strings.Join(failed, "; "))
	}
	return nil
}

func fetchOne(ctx context.Context, open Opener, root string, t Template) error {
	resp, err := open(ctx, rawBase+ReviewedTag+"/"+t.Path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the server answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTemplateSize+1))
	if err != nil || len(body) > maxTemplateSize {
		return errors.New("the file could not be read or is too large")
	}
	if checksum(body) != strings.ToLower(t.SHA256) {
		return errors.New("it is not the file that was reviewed")
	}
	dest := filepath.Join(root, filepath.Clean(t.Path))
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".tpl-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}
