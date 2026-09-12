package network

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLLMChatTransportForcesHTTP1 verifies the LLM chat transports never
// negotiate HTTP/2. NVIDIA's integrate.api.nvidia.com has a broken HTTP/2 path
// for chat completions (curl: "Error in the HTTP2 framing layer"; Go:
// "unexpected EOF" on POST) while its HTTP/1.1 path is stable, so the chat
// client must stay on HTTP/1.1. The canonical way to keep a custom transport
// on HTTP/1.1 is a non-nil empty TLSNextProto plus ForceAttemptHTTP2=false.
// SharedTransport is deliberately excluded: it serves provider infrastructure
// (catalogue listing, connection tests) where HTTP/2 is fine.
func TestLLMChatTransportForcesHTTP1(t *testing.T) {
	transports := map[string]*http.Transport{
		"local": LLMChatTransport,
		"cloud": CloudLLMChatTransport,
	}
	for name, tr := range transports {
		t.Run(name, func(t *testing.T) {
			if tr.ForceAttemptHTTP2 {
				t.Errorf("%s: ForceAttemptHTTP2 must be false", name)
			}
			if tr.TLSNextProto == nil {
				t.Errorf("%s: TLSNextProto must be non-nil (empty map disables HTTP/2)", name)
			}
			if len(tr.TLSNextProto) != 0 {
				t.Errorf("%s: TLSNextProto must be empty, got %d entries", name, len(tr.TLSNextProto))
			}
		})
	}
}

// TestCloudLLMChatTransportShorterHeaderTimeout locks the cloud transport's
// response-header timeout below the local one: NVIDIA's free-tier gateway holds
// a saturated request ~60s then drops the connection (unexpected EOF), so the
// cloud client bounds the wait at 45s to classify the failure as a clean
// client-side timeout and let the retry fire sooner.
func TestCloudLLMChatTransportShorterHeaderTimeout(t *testing.T) {
	if CloudLLMChatTransport.ResponseHeaderTimeout >= LLMChatTransport.ResponseHeaderTimeout {
		t.Errorf("cloud header timeout (%v) must be shorter than local (%v)",
			CloudLLMChatTransport.ResponseHeaderTimeout, LLMChatTransport.ResponseHeaderTimeout)
	}
	if CloudLLMChatTransport.ResponseHeaderTimeout != 45*time.Second {
		t.Errorf("cloud header timeout = %v, want 45s", CloudLLMChatTransport.ResponseHeaderTimeout)
	}
}

// The sandboxing plan's single-stream claim ("every agent exec site is one of
// two Wrap points; every egress path rides an injected transport") must stay a
// TESTED fact. This audit parses every non-test Go file and fails on:
//  1. http.DefaultClient (any use), http.Get/Post/PostForm/Head convenience
//     calls, or an http.Client{...} without an explicit Transport (which
//     silently uses http.DefaultTransport — the same raw egress);
//  2. exec.Command/CommandContext outside the allowlisted infra spawn sites
//     (agent-triggered children may only be spawned by the shell factory and
//     executeLocal, which receive the sandbox Wrap);
//  3. os.StartProcess anywhere.
//
// AST-based on purpose: the previous line-based scan could be bypassed by
// splitting a call across lines, aliasing the import, or a comment containing
// "//". Add a new site to execAllowlist ONLY when it is genuinely
// infrastructure (model-server lifecycle, metrics, procwatch) and never
// agent-triggered.
var execAllowlist = []string{
	"internal/shell/terminal.go",                    // persistent shell spawn (Wrap point)
	"internal/core/tools/terminal.go",               // executeLocal spawn (Wrap point)
	"internal/core/llm/providers/local_provider.go", // llama-server lifecycle cleanup
	"internal/platform/metrics/gpu_providers.go",    // nvidia-smi / GPU sampling
	"internal/platform/process/",                    // procwatch ps / rlimit probes
	"internal/testing/utils/",                       // ExecCommand test seam
}

// httpConvenienceEgress are the package-level helpers that dial with the
// default transport.
var httpConvenienceEgress = map[string]bool{
	"Get": true, "Post": true, "PostForm": true, "Head": true,
}

func findBackendRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("backend root (go.mod) not found above cwd")
		}
		dir = parent
	}
}

func TestAuditNoOutOfBandEgress(t *testing.T) {
	root := findBackendRoot(t)
	fset := token.NewFileSet()
	var violations []string

	report := func(pos token.Pos, format string, args ...any) {
		p := fset.Position(pos)
		rel, err := filepath.Rel(root, p.Filename)
		if err != nil {
			rel = p.Filename
		}
		violations = append(violations, fmt.Sprintf("%s:%d: %s", rel, p.Line, fmt.Sprintf(format, args...)))
	}

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "frontend_dist" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.SelectorExpr:
				if isPkgIdent(node.X, "http") && node.Sel.Name == "DefaultClient" {
					report(node.Pos(), "http.DefaultClient (use an injected transport)")
				}
			case *ast.CallExpr:
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch {
				case isPkgIdent(sel.X, "http") && httpConvenienceEgress[sel.Sel.Name]:
					report(node.Pos(), "http.%s convenience call (use an injected transport)", sel.Sel.Name)
				case isPkgIdent(sel.X, "exec") && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext"):
					if !isExecAllowed(rel) {
						report(node.Pos(), "exec.%s outside the allowlisted infra spawn sites", sel.Sel.Name)
					}
				case isPkgIdent(sel.X, "os") && sel.Sel.Name == "StartProcess":
					report(node.Pos(), "os.StartProcess (uncontrolled spawn)")
				}
			case *ast.CompositeLit:
				if clientLitWithoutTransport(node) {
					report(node.Pos(), "http.Client without an explicit Transport (defaults to raw http.DefaultTransport)")
				}
			}
			return true
		})
		return nil
	})

	if len(violations) > 0 {
		t.Errorf("out-of-band egress/spawn found (Constitution I.1 + sandbox single-stream):\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// isPkgIdent reports whether e is the bare package identifier name.
func isPkgIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// clientLitWithoutTransport reports an http.Client composite literal that does
// not set a Transport field (so it falls back to http.DefaultTransport).
func clientLitWithoutTransport(lit *ast.CompositeLit) bool {
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || !isPkgIdent(sel.X, "http") || sel.Sel.Name != "Client" {
		return false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Transport" {
			return false
		}
	}
	return true
}

func isExecAllowed(rel string) bool {
	for _, a := range execAllowlist {
		if strings.HasPrefix(rel, a) {
			return true
		}
	}
	return false
}
