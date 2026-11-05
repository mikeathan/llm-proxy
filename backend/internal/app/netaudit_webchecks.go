package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/nuclei"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/progexec"
)

// templatesFolder is where the reviewed web-check templates live, inside the program's own data folder and outside every workspace.
const templatesFolder = "netaudit-web-templates"

// webChecks is the nightly switch and the checker (nuclei under the guarded-program rules), plus the templates it runs.
type webChecks struct {
	Config    *netaudit.WebChecksConfig
	Checker   *nuclei.Checker
	Templates *webTemplates
}

func (s *AppServices) newWebChecks(store netaudit.PolicyStore, runner progexec.Runner, netTools *tools.NetworkTools) webChecks {
	root := filepath.Join(s.AppCtx.RootDir(), templatesFolder)
	s.AppCtx.AfterFactoryReset("The downloaded web-check templates", func() error { return os.RemoveAll(root) })
	checker := nuclei.NewChecker(runner, root)
	return webChecks{
		Config: netaudit.NewWebChecksConfig(store), Checker: checker,
		Templates: &webTemplates{checker: checker, root: root, open: netTools.Open},
	}
}

// webTemplates is what the handlers use: the status and the operator's download of the reviewed templates, through the guarded client.
type webTemplates struct {
	checker *nuclei.Checker
	root    string
	open    nuclei.Opener
}

func (w *webTemplates) Ready() (bool, string) { return w.checker.Ready() }

func (w *webTemplates) Status() nuclei.Status { return nuclei.StatusOf(w.root, nuclei.Reviewed) }

func (w *webTemplates) Download(ctx context.Context) error {
	if err := nuclei.Download(ctx, w.open, w.root, nuclei.Reviewed); err != nil {
		return fmt.Errorf("the web-check templates could not be downloaded: %w", err)
	}
	return nil
}
