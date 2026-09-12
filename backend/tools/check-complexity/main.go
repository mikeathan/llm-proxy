// check-complexity computes McCabe cyclomatic complexity for every function
// in the project and fails if any exceeds the threshold. Uses only stdlib —
// no external dependencies.
//
// The scanned tree is anchored at the Go module root (found by walking up to
// go.mod from the working directory) so the documented invocation
// (`go run ./tools/check-complexity/` from backend/) actually analyzes
// backend/internal instead of silently walking backend/backend/internal.
//
// Baseline: the repository carries pre-existing functions above the limit
// (recorded in knownComplexityExceptions). Those are reported as debt but do
// not fail the gate; a function NOT in the baseline that exceeds the limit
// fails CI. When a baseline entry drops to <= threshold it should be removed
// from the map (the tool prints stale entries to make that visible).
//
// `go run ./tools/check-complexity/ -baseline` prints the exact keys for
// functions currently over the limit (to refresh the map).
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultThreshold = 12

// knownComplexityExceptions is the pre-existing complexity debt baseline,
// keyed "internal/path/file.go:FuncName". Do NOT add new entries: refactor the
// function instead (repo rule: complexity ≤ 12).
var knownComplexityExceptions = map[string]bool{
	"internal/app/app_context_system.go:ApplySystemUpdate":                                       true,
	"internal/app/playback_bridge.go:Stream":                                                     true,
	"internal/core/assistant/agent.go:ApplyModelConfig":                                          true,
	"internal/core/assistant/agent.go:applyDefaults":                                             true,
	"internal/core/assistant/conversation_helpers.go:TruncateHistory":                            true,
	"internal/core/assistant/conversation_helpers.go:buildPartialHistory":                        true,
	"internal/core/assistant/failures/failure.go:classifyHTTPFailure":                            true,
	"internal/core/assistant/guardrails/guardrails.go:PersistOverride":                           true,
	"internal/core/assistant/prompts/templates.go:formatToolParameters":                          true,
	"internal/core/assistant/react_strategy.go:Run":                                              true,
	"internal/core/assistant/session.go:executeTurn":                                             true,
	"internal/core/assistant/session.go:finalizeReport":                                          true,
	"internal/core/assistant/session.go:handleNoToolCalls":                                       true,
	"internal/core/assistant/session.go:handleToolTurn":                                          true,
	"internal/core/assistant/session.go:synthesizeRunSummary":                                    true,
	"internal/core/assistant/stream.go:computeNextResponse":                                      true,
	"internal/core/assistant/stream.go:computeNextResponseNonStreaming":                          true,
	"internal/core/assistant/stream.go:processStream":                                            true,
	"internal/core/assistant/tool_exec.go:executePlan":                                           true,
	"internal/core/assistant/tool_exec.go:generatePlanContent":                                   true,
	"internal/core/assistant/tool_exec.go:processToolCalls":                                      true,
	"internal/core/assistant/tool_exec.go:sanitizeToolArgs":                                      true,
	"internal/core/assistant/tool_exec.go:validateToolArgs":                                      true,
	"internal/core/automation/broadcast.go:Publish":                                              true,
	"internal/core/automation/dispatcher.go:executeAutomation":                                   true,
	"internal/core/llm/manager.go:ApplyModelOverrides":                                           true,
	"internal/core/llm/manager.go:GetInstance":                                                   true,
	"internal/core/llm/manager.go:ReconcileLocalServingContext":                                  true,
	"internal/core/llm/providers/provider_openai_compatible.go:fetchModels":                      true,
	"internal/core/llm/providers/provider_openai_compatible.go:probeNativeToolsOnce":             true,
	"internal/core/llm/providers/registrar.go:Build":                                             true,
	"internal/core/llm/providers/registrar.go:EffectiveEndpointLocked":                           true,
	"internal/core/proxy/client.go:Stream":                                                       true,
	"internal/core/proxy/client.go:classifyTransportError":                                       true,
	"internal/core/proxy/client.go:doRequest":                                                    true,
	"internal/core/proxy/history.go:NormalizeHistory":                                            true,
	"internal/core/tools/filesystem.go:validateFilePath":                                         true,
	"internal/core/tools/manifests.go:LoadManifestAsTool":                                        true,
	"internal/core/tools/memory_tools.go:Search":                                                 true,
	"internal/core/tools/network.go:FetchURL":                                                    true,
	"internal/core/tools/network.go:ScanLocalNetwork":                                            true,
	"internal/core/tools/network.go:resolveAndQueueTargets":                                      true,
	"internal/core/tools/terminal.go:checkPathSecurity":                                          true,
	"internal/core/tools/terminal.go:executeShell":                                               true,
	"internal/core/tools/terminal.go:parseHeredocMarker":                                         true,
	"internal/core/tools/terminal.go:scanCommandSegments":                                        true,
	"internal/platform/memory/fts.go:sanitiseFTSQuery":                                           true,
	"internal/platform/metrics/gpu_providers.go:buildGPUProvider":                                true,
	"internal/platform/metrics/gpu_providers.go:findNestedMemory":                                true,
	"internal/platform/metrics/gpu_providers.go:parseAmdGpuTopJSON":                              true,
	"internal/platform/persistence/workspace.go:DeleteRunByID":                                   true,
	"internal/platform/storage/manager.go:mergeAppConfigDefaults":                                true,
	"internal/platform/storage/reset.go:FactoryReset":                                            true,
	"internal/recordings/playback.go:NewPlaybackClient":                                          true,
	"internal/shell/terminal.go:Execute":                                                         true,
	"internal/shell/terminal.go:prepareShellEnv":                                                 true,
	"internal/testing/llmprofiles/profiles.go:parseFixture":                                      true,
	"internal/transport/http/handlers/admin_handlers.go:AdminConnectorWebhookHandler":            true,
	"internal/transport/http/handlers/admin_view.go:getModelsView":                               true,
	"internal/transport/http/handlers/model_handlers.go:enrichMetadataFromProviders":             true,
	"internal/transport/http/handlers/model_handlers.go:handleAddModel":                          true,
	"internal/transport/http/handlers/model_handlers.go:handleUpdateModel":                       true,
	"internal/transport/http/handlers/model_handlers.go:hasModelOverrides":                       true,
	"internal/transport/http/handlers/model_handlers.go:resolvePublishedCapabilitiesFromCatalog": true,
	"internal/transport/http/handlers/registry_handlers.go:discoverModelFiles":                   true,
	"internal/transport/http/handlers/secrets_handlers.go:AdminProviderKeysPutHandler":           true,
	"internal/transport/http/handlers/webhook_handlers.go:ServeHTTP":                             true,
	"internal/transport/http/handlers/webhook_handlers.go:handleAutomation":                      true,
}

// moduleRoot returns the nearest ancestor directory containing go.mod.
func moduleRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

func main() {
	baseline := flag.Bool("baseline", false, "print keys for every function over the limit and exit 0")
	flag.Parse()
	threshold := defaultThreshold

	root := moduleRoot()
	abs := filepath.Join(root, "internal")

	type violation struct {
		key string
		msg string
	}
	var newViolations []violation
	var debt []string
	seen := map[string]bool{}

	walkErr := filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			c := complexity(fn.Body)
			if c <= threshold {
				continue
			}
			key := rel + ":" + fn.Name.Name
			seen[key] = true
			line := fset.Position(fn.Pos()).Line
			msg := fmt.Sprintf("%s:%d: %s: cyclomatic complexity %d exceeds limit %d", rel, line, fn.Name.Name, c, threshold)
			if knownComplexityExceptions[key] {
				debt = append(debt, msg)
				continue
			}
			newViolations = append(newViolations, violation{key: key, msg: msg})
		}
		return nil
	})
	if walkErr != nil {
		fmt.Fprintf(os.Stderr, "walk %s: %v\n", abs, walkErr)
		os.Exit(1)
	}

	sort.Strings(debt)
	for _, m := range debt {
		fmt.Println("known debt:", m)
	}

	if *baseline {
		keys := make([]string, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("\t%q: true,\n", k)
		}
		return
	}

	// Stale baseline entries (fixed or moved) are noise; surface them so the
	// map shrinks over time.
	var stale []string
	for key := range knownComplexityExceptions {
		if !seen[key] {
			stale = append(stale, key)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		fmt.Fprintf(os.Stderr, "check-complexity: %d stale baseline entr(ies) — remove after refactor: %s\n",
			len(stale), strings.Join(stale, ", "))
	}

	if len(newViolations) > 0 {
		for _, v := range newViolations {
			fmt.Println("FAIL:", v.msg)
		}
		fmt.Fprintf(os.Stderr, "check-complexity: %d NEW function(s) over complexity %d (refactor them; do not extend the baseline)\n",
			len(newViolations), threshold)
		os.Exit(1)
	}
	fmt.Printf("check-complexity: OK — %d baseline function(s) over complexity %d (known debt), no new violations\n",
		len(debt), threshold)
}

// complexity returns the McCabe cyclomatic complexity for a block.
func complexity(block *ast.BlockStmt) int {
	c := 1
	ast.Inspect(block, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt:
			c++
		case *ast.ForStmt:
			c++
		case *ast.RangeStmt:
			c++
		case *ast.CaseClause:
			c++
		case *ast.CommClause:
			c++
		case *ast.BinaryExpr:
			be := n.(*ast.BinaryExpr)
			if be.Op == token.LAND || be.Op == token.LOR {
				c++
			}
		}
		return true
	})
	return c
}
