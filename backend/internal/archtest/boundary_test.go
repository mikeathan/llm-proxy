// Package archtest holds the fitness functions of Constitution IV.6: tests that fail the build when a stack breaches its boundary.
// It has no production code.
package archtest

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The service is the module "llm-proxy" (backend/). The SDK and each stack are separate Go modules under the repository's stacks/
// folder, named llm-proxy-stacks/<name>, so the compiler already hides llm-proxy/internal and llm-proxy/models from them; these
// tests check the declared dependencies and the rules the compiler cannot (ADR-0032).
const (
	module     = "llm-proxy"
	sdkPath    = "llm-proxy-stacks/sdk"
	stacksRoot = "llm-proxy-stacks/"
	appPath    = module + "/internal/app"
)

type pkg struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	Imports    []string
	TestImport []string `json:"TestImports"`
	XTest      []string `json:"XTestImports"`
}

func (p pkg) allImports() []string {
	return append(append(append([]string{}, p.Imports...), p.TestImport...), p.XTest...)
}

func isStdlib(path string) bool {
	first := strings.SplitN(path, "/", 2)[0]
	return !strings.Contains(first, ".") && first != module && first != strings.TrimSuffix(stacksRoot, "/")
}

func stackOf(importPath string) string {
	rest := strings.TrimPrefix(importPath, stacksRoot)
	return strings.SplitN(rest, "/", 2)[0]
}

// stackImportViolations lists the imports a stack package may not have: only the standard library, third-party libraries, the SDK and
// the stack's own sub-packages.
func stackImportViolations(p pkg) []string {
	var bad []string
	for _, imp := range p.allImports() {
		switch {
		case imp == sdkPath, isStdlib(imp):
		case strings.HasPrefix(imp, stacksRoot) && stackOf(imp) == stackOf(p.ImportPath):
		case imp == module || strings.HasPrefix(imp, module+"/"), strings.HasPrefix(imp, stacksRoot):
			bad = append(bad, p.ImportPath+" imports "+imp)
		}
	}
	return bad
}

// sdkImportViolations: the SDK is contracts only and imports nothing of ours or of third parties.
func sdkImportViolations(p pkg) []string {
	var bad []string
	for _, imp := range p.allImports() {
		if !isStdlib(imp) {
			bad = append(bad, p.ImportPath+" imports "+imp)
		}
	}
	return bad
}

// isStackImport reports whether an import is a stack. The SDK is the contract the core shares with stacks, not a stack.
func isStackImport(imp string) bool {
	return strings.HasPrefix(imp, stacksRoot) && imp != sdkPath
}

// coreImportsOfStacks lists the packages outside stacks/ that import a stack. Only internal/app may, and only through stacks.go.
func coreImportsOfStacks(p pkg) []string {
	if strings.HasPrefix(p.ImportPath, stacksRoot) || strings.HasSuffix(p.ImportPath, "/archtest") {
		return nil
	}
	var bad []string
	for _, imp := range p.allImports() {
		if isStackImport(imp) && p.ImportPath != appPath {
			bad = append(bad, p.ImportPath+" imports "+imp+"; only internal/app/stacks.go may")
		}
	}
	return bad
}

// backendDir is the service module's directory: the nearest ancestor of the test's working directory that holds a go.mod.
func backendDir(t *testing.T) string {
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
			t.Fatal("no go.mod above the test")
		}
		dir = parent
	}
}

// stackModuleDirs are the directories under <repo>/stacks that hold a Go module: the SDK and every stack.
func stackModuleDirs(t *testing.T) []string {
	t.Helper()
	root := filepath.Join(filepath.Dir(backendDir(t)), "stacks")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("no stacks folder at %s: %v", root, err)
	}
	var dirs []string
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(root, e.Name(), "go.mod")); e.IsDir() && err == nil {
			dirs = append(dirs, filepath.Join(root, e.Name()))
		}
	}
	return dirs
}

// goIn runs the go tool in a stack module as that module alone (no workspace), the way CI builds it, so a stack is judged by its own
// go.mod and not by whatever a developer's local go.work adds.
func goIn(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Fatalf("go %s in %s: %v\n%s", strings.Join(args, " "), dir, err, ee.Stderr)
		}
		t.Fatalf("go %s in %s: %v", strings.Join(args, " "), dir, err)
	}
	return out
}

func listIn(t *testing.T, dir string) []pkg {
	t.Helper()
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list in %s: %v", dir, err)
	}
	return decodePkgs(t, out)
}

func decodePkgs(t *testing.T, out []byte) []pkg {
	t.Helper()
	var pkgs []pkg
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// goList lists the packages of the service and of every stack module.
func goList(t *testing.T) []pkg {
	t.Helper()
	pkgs := listIn(t, backendDir(t))
	for _, d := range stackModuleDirs(t) {
		pkgs = append(pkgs, decodePkgs(t, goIn(t, d, "list", "-json", "./..."))...)
	}
	return pkgs
}

// moduleFile is what `go mod edit -json` reports of a go.mod.
type moduleFile struct {
	Module  struct{ Path string }
	Require []struct{ Path string }
	Replace []struct{ Old, New struct{ Path string } }
}

// moduleViolations are the go.mod entries a stack may not have: it may require the SDK and third-party libraries, never the service
// or another stack, and may replace only the SDK with the sibling folder. The SDK requires nothing.
func moduleViolations(m moduleFile) []string {
	var bad []string
	isSDK := m.Module.Path == sdkPath
	for _, r := range m.Require {
		switch {
		case isSDK:
			bad = append(bad, m.Module.Path+" requires "+r.Path+"; the SDK requires nothing")
		case r.Path == sdkPath:
		case r.Path == module, strings.HasPrefix(r.Path, module+"/"), strings.HasPrefix(r.Path, stacksRoot):
			bad = append(bad, m.Module.Path+" requires "+r.Path)
		}
	}
	for _, r := range m.Replace {
		if r.Old.Path != sdkPath || r.New.Path != "../sdk" {
			bad = append(bad, m.Module.Path+" replaces "+r.Old.Path+" with "+r.New.Path+"; only the SDK may be replaced, with ../sdk")
		}
	}
	return bad
}

func readModuleFile(t *testing.T, dir string) moduleFile {
	t.Helper()
	var m moduleFile
	if err := json.Unmarshal(goIn(t, dir, "mod", "edit", "-json"), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// ---- the rules, applied to the real tree ----

func TestStacksImportOnlyTheSDK(t *testing.T) {
	seen := 0
	for _, p := range goList(t) {
		switch {
		case p.ImportPath == sdkPath:
			for _, v := range sdkImportViolations(p) {
				t.Error(v)
			}
		case strings.HasPrefix(p.ImportPath, stacksRoot):
			seen++
			for _, v := range stackImportViolations(p) {
				t.Error(v)
			}
		default:
			for _, v := range coreImportsOfStacks(p) {
				t.Error(v)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no stack package found: the test would pass over nothing")
	}
}

// A stack's go.mod is the compiler's version of the same rule: a dependency it does not list cannot be imported.
func TestStackModulesDeclareOnlyTheSDK(t *testing.T) {
	dirs := stackModuleDirs(t)
	if len(dirs) < 2 {
		t.Fatalf("expected the SDK and at least one stack under stacks/, found %v", dirs)
	}
	for _, d := range dirs {
		for _, v := range moduleViolations(readModuleFile(t, d)) {
			t.Error(v)
		}
	}
}

// Within internal/app, stacks.go is the only file that imports a stack.
func TestOnlyTheCompositionRootImportsAStack(t *testing.T) {
	for _, p := range goList(t) {
		if p.ImportPath != appPath {
			continue
		}
		for _, f := range p.GoFiles {
			if f == "stacks.go" {
				continue
			}
			af, err := parser.ParseFile(token.NewFileSet(), filepath.Join(p.Dir, f), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, im := range af.Imports {
				if path, _ := strconv.Unquote(im.Path.Value); isStackImport(path) {
					t.Errorf("%s imports %s; only stacks.go may", f, path)
				}
			}
		}
	}
}

// ---- the rules refuse a breach (the tests above would otherwise pass over nothing) ----

func TestRulesRefuseABreach(t *testing.T) {
	stack := pkg{ImportPath: stacksRoot + "net", Imports: []string{"fmt", "github.com/x/y", sdkPath, stacksRoot + "net/oui"}}
	if v := stackImportViolations(stack); len(v) != 0 {
		t.Errorf("a clean stack was refused: %v", v)
	}
	for _, imp := range []string{module + "/internal/platform/sandbox", module + "/models", module + "/utils", stacksRoot + "agents"} {
		p := stack
		p.Imports = append(append([]string{}, stack.Imports...), imp)
		if len(stackImportViolations(p)) == 0 {
			t.Errorf("a stack importing %s must be refused", imp)
		}
	}
	p := stack
	p.XTest = []string{module + "/internal/core/tools"}
	if len(stackImportViolations(p)) == 0 {
		t.Error("a stack's external test importing the core must be refused")
	}
	if len(sdkImportViolations(pkg{ImportPath: sdkPath, Imports: []string{"fmt", module + "/models"}})) == 0 {
		t.Error("an SDK importing the core must be refused")
	}
	if len(sdkImportViolations(pkg{ImportPath: sdkPath, Imports: []string{"github.com/x/y"}})) == 0 {
		t.Error("an SDK importing a third-party library must be refused")
	}
	if len(coreImportsOfStacks(pkg{ImportPath: module + "/internal/core/tools", Imports: []string{stacksRoot + "net"}})) == 0 {
		t.Error("the core importing a stack must be refused")
	}
	if len(coreImportsOfStacks(pkg{ImportPath: appPath, Imports: []string{stacksRoot + "net"}})) != 0 {
		t.Error("internal/app may import a stack")
	}
	if len(coreImportsOfStacks(pkg{ImportPath: module + "/internal/core/tools", Imports: []string{stacksRoot + "network/oui"}})) == 0 {
		t.Error("a core package may not import a stack package")
	}
}

func TestModuleRulesRefuseABreach(t *testing.T) {
	ok := moduleFile{}
	ok.Module.Path = stacksRoot + "net"
	ok.Require = append(ok.Require, struct{ Path string }{sdkPath}, struct{ Path string }{"github.com/x/y"})
	if v := moduleViolations(ok); len(v) != 0 {
		t.Errorf("a clean stack module was refused: %v", v)
	}
	for _, req := range []string{module, module + "/models", stacksRoot + "agents"} {
		m := ok
		m.Require = append(append([]struct{ Path string }{}, ok.Require...), struct{ Path string }{req})
		if len(moduleViolations(m)) == 0 {
			t.Errorf("a stack requiring %s must be refused", req)
		}
	}
	bad := ok
	bad.Replace = []struct{ Old, New struct{ Path string } }{{}}
	bad.Replace[0].Old.Path, bad.Replace[0].New.Path = module, "../../backend"
	if len(moduleViolations(bad)) == 0 {
		t.Error("a stack replacing the service with a local folder must be refused")
	}
	sdk := moduleFile{}
	sdk.Module.Path = sdkPath
	sdk.Require = []struct{ Path string }{{"github.com/x/y"}}
	if len(moduleViolations(sdk)) == 0 {
		t.Error("an SDK that requires anything must be refused")
	}
}

// ---- a stack cannot dial or start a program without the SDK ----

// forbidden are the calls that reach the network or start a program without the guard or the isolation library. A stack gets
// connections only from stacksdk.Guard and runs programs only through stacksdk.Isolation (Constitution I.1 to I.3, II.3, ADR-0033).
var forbidden = map[string][]string{
	"net":      {"Dial", "DialTimeout", "DialTCP", "DialUDP", "DialIP", "DialUnix", "Listen", "ListenTCP", "ListenUDP", "ListenPacket", "ListenIP", "ListenUnix", "ListenMulticastUDP", "Dialer", "ListenConfig"},
	"net/http": {"Get", "Post", "Head", "PostForm", "DefaultClient", "DefaultTransport", "ListenAndServe", "ListenAndServeTLS", "Serve", "Transport"},
	"os/exec":  {"Command", "CommandContext", "Cmd"},
	"os":       {"StartProcess", "Exec"},
	"syscall":  {"Exec", "ForkExec", "StartProcess", "Socket", "Connect", "RawSyscall", "Syscall", "RawSyscall6", "Syscall6"},
	"unsafe":   {"Pointer"},
}

// bypasses lists every use of a forbidden name in one Go file.
func bypasses(filename string, src any) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	if err != nil {
		return nil, err
	}
	local := map[string]string{} // the name an import is used by -> its path
	for _, im := range f.Imports {
		path, _ := strconv.Unquote(im.Path.Value)
		if _, watched := forbidden[path]; !watched {
			continue
		}
		name := filepath.Base(path)
		if im.Name != nil {
			name = im.Name.Name
		}
		local[name] = path
	}
	var found []string
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		for _, banned := range forbidden[local[id.Name]] {
			if sel.Sel.Name == banned && local[id.Name] != "" {
				found = append(found, filename+": "+local[id.Name]+"."+banned)
			}
		}
		return true
	})
	return found, nil
}

func TestStacksCannotDialOrStartAProgramWithoutTheSDK(t *testing.T) {
	files := 0
	for _, p := range goList(t) {
		if !strings.HasPrefix(p.ImportPath, stacksRoot) {
			continue
		}
		for _, name := range p.GoFiles {
			files++
			found, err := bypasses(filepath.Join(p.Dir, name), nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range found {
				t.Errorf("%s: use stacksdk.Guard or stacksdk.Isolation", f)
			}
		}
	}
	if files == 0 {
		t.Fatal("no stack source found: the test would pass over nothing")
	}
}

func TestTheDialScanCatchesTheWaysAroundTheGuard(t *testing.T) {
	cases := map[string]string{
		"net.Dial":         `package s; import "net"; func f() { net.Dial("tcp", "x") }`,
		"aliased net":      `package s; import n "net"; func f() { _ = n.Dialer{} }`,
		"listen":           `package s; import "net"; func f() { net.Listen("tcp", ":1") }`,
		"http.Get":         `package s; import "net/http"; func f() { http.Get("x") }`,
		"default client":   `package s; import "net/http"; var c = http.DefaultClient`,
		"exec.Command":     `package s; import "os/exec"; func f() { exec.Command("ls") }`,
		"syscall.ForkExec": `package s; import "syscall"; func f() { syscall.ForkExec("x", nil, nil) }`,
		"os.StartProcess":  `package s; import "os"; func f() { os.StartProcess("x", nil, nil) }`,
		"raw socket":       `package s; import "syscall"; func f() { syscall.Socket(1, 2, 3) }`,
	}
	for name, src := range cases {
		if found, err := bypasses("x.go", src); err != nil || len(found) == 0 {
			t.Errorf("%s: not caught (%v, %v)", name, found, err)
		}
	}
	fine := `package s; import ("net"; "net/http"; "net/netip"); func f(c *http.Client, ip net.IP) { _ = netip.Addr{}; _ = c; _ = ip }`
	if found, err := bypasses("x.go", fine); err != nil || len(found) != 0 {
		t.Errorf("types and values that cannot open a connection must pass: %v %v", found, err)
	}
}

// A child of the service shares its uid, gid and cgroup, so the only thing that keeps it from a root helper's socket is the
// Unix-socket filter the sandbox runner applies (design network-privileged-helper-design.md section 2, ADR-0034). That filter is applied by
// sandbox.Provider.Wrap, so the claim "every child is filtered" is a claim about the places the service starts a program. These tests
// pin those places exactly: a new one fails the build until it is added below with its reason, and every entry that says "wrapped"
// must still call Wrap in the function it names.

// spawnSite is one place the service's own code starts, or builds the command for, a child process.
type spawnSite struct {
	file, fn, what string
	// wrappedIn names the function in the same file that calls Wrap before the command starts; "" means the site is not wrapped and
	// reason says why that is safe.
	wrappedIn string
	reason    string
}

const agentCommand = "process.AgentCommand"

var spawnSites = []spawnSite{
	{file: "internal/core/tools/terminal.go", fn: "(*TerminalTools).newCommand", what: agentCommand, wrappedIn: "(*TerminalTools).executeLocal"},
	{file: "internal/shell/terminal.go", fn: "newPersistentShell", what: agentCommand, wrappedIn: "newPersistentShell"},
	{file: "internal/platform/progexec/progexec.go", fn: "Runner.Run", what: agentCommand, wrappedIn: "Runner.Run"},
	{file: "internal/platform/progexec/progexec.go", fn: "Detect", what: agentCommand,
		reason: "asks an installed program for its version with fixed arguments and an empty environment; no agent text reaches it"},
	{file: "internal/platform/process/agent_command.go", fn: "AgentCommand", what: "exec.CommandContext",
		reason: "the one constructor of agent children; each caller above wraps, which the entries above check"},
	{file: "internal/platform/sandbox/runner.go", fn: "RunChildIfRequested", what: "syscall.Exec",
		reason: "the runner replaces itself with the real program after it has applied the filter"},
	{file: "internal/platform/metrics/gpu_providers.go", fn: "defaultCommandRunner", what: "exec.CommandContext",
		reason: "fixed GPU query commands the service starts for itself; no agent input"},
	{file: "internal/platform/process/process_darwin.go", fn: "listByBinary", what: "exec.Command",
		reason: "fixed ps command on macOS, where the helper does not exist"},
	{file: "internal/testing/utils/types.go", fn: "", what: "exec.Command",
		reason: "package variable tests replace; no non-test caller"},
	{file: "internal/testing/utils/types.go", fn: "", what: "exec.CommandContext",
		reason: "package variable tests replace; no non-test caller"},
}

const (
	execPath    = "os/exec"
	osPath      = "os"
	syscallPath = "syscall"
	processPath = module + "/internal/platform/process"
)

// spawnNames are the ways to start a program: exec.Command, os.StartProcess, syscall.Exec, ForkExec and StartProcess, and the core's
// own constructor process.AgentCommand.
var spawnNames = map[string][]string{
	execPath:    {"Command", "CommandContext"},
	osPath:      {"StartProcess"},
	syscallPath: {"Exec", "ForkExec", "StartProcess"},
	processPath: {"AgentCommand"},
}

// funcUses is what one function (or the package-level declarations, fn "") does: the spawn names it uses and whether it calls a method
// named Wrap.
type funcUses struct {
	spawns []string
	wraps  bool
}

func funcName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	switch r := d.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := r.X.(*ast.Ident); ok {
			return "(*" + id.Name + ")." + d.Name.Name
		}
	case *ast.Ident:
		return r.Name + "." + d.Name.Name
	}
	return d.Name.Name
}

// spawnUses reports, per function of one Go file, which ways of starting a program it names and whether it calls Wrap.
func spawnUses(filename string, src any) (map[string]*funcUses, error) {
	f, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	if err != nil {
		return nil, err
	}
	local := map[string]string{}
	for _, im := range f.Imports {
		path, _ := strconv.Unquote(im.Path.Value)
		if _, watched := spawnNames[path]; !watched {
			continue
		}
		name := filepath.Base(path)
		if im.Name != nil {
			name = im.Name.Name
		}
		local[name] = path
	}
	out := map[string]*funcUses{}
	scan := func(fn string, root ast.Node) {
		ast.Inspect(root, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			u := out[fn]
			if u == nil {
				u = &funcUses{}
				out[fn] = u
			}
			if sel.Sel.Name == "Wrap" {
				u.wraps = true
			}
			if id, ok := sel.X.(*ast.Ident); ok && local[id.Name] != "" && slices.Contains(spawnNames[local[id.Name]], sel.Sel.Name) {
				u.spawns = append(u.spawns, id.Name+"."+sel.Sel.Name)
			}
			return true
		})
	}
	for _, decl := range f.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok {
			scan(funcName(d), d)
		} else {
			scan("", decl)
		}
	}
	return out, nil
}

// serviceSources lists the service's non-test Go files, as paths relative to backend/. The dev tools, this package and generated output
// are not the service.
func serviceSources(t *testing.T) (root string, files []string) {
	t.Helper()
	root = backendDir(t)
	skip := map[string]bool{"tools": true, "graphify-out": true, "data": true} // top-level folders only: internal/core/tools is the service
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if skip[rel] || d.Name() == "testdata" || rel == filepath.Join("internal", "archtest") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return root, files
}

func (s spawnSite) key() string { return s.file + " " + s.fn + " " + s.what }

func TestEveryPlaceTheServiceStartsAProgramIsPinned(t *testing.T) {
	root, files := serviceSources(t)
	found := map[string]bool{}
	wraps := map[string]bool{}
	for _, rel := range files {
		uses, err := spawnUses(filepath.Join(root, rel), nil)
		if err != nil {
			t.Fatal(err)
		}
		for fn, u := range uses {
			if u.wraps {
				wraps[rel+" "+fn] = true
			}
			for _, what := range u.spawns {
				found[(spawnSite{file: rel, fn: fn, what: what}).key()] = true
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("no spawn site found: the test would pass over nothing")
	}
	pinned := map[string]bool{}
	for _, s := range spawnSites {
		pinned[s.key()] = true
		switch {
		case !found[s.key()]:
			t.Errorf("%s: pinned but not found; remove the entry if the code moved", s.key())
		case s.wrappedIn != "" && !wraps[s.file+" "+s.wrappedIn]:
			t.Errorf("%s: must be wrapped by %s, which no longer calls Wrap (the child would run without the Unix-socket filter)", s.key(), s.wrappedIn)
		case s.wrappedIn == "" && s.reason == "":
			t.Errorf("%s: an unwrapped site needs a reason", s.key())
		}
	}
	for k := range found {
		if !pinned[k] {
			t.Errorf("%s: a new way to start a program; add it to spawnSites with wrappedIn or a reason (a child of the service must be filtered, or unreachable by the agent)", k)
		}
	}
}

func TestTheSpawnScanSeesTheWaysToStartAProgram(t *testing.T) {
	cases := map[string]string{
		"exec.Command":     `package s; import "os/exec"; func f() { exec.Command("ls") }`,
		"aliased":          `package s; import e "os/exec"; func f() { e.CommandContext(nil, "ls") }`,
		"as a value":       `package s; import "os/exec"; var c = exec.Command`,
		"syscall.ForkExec": `package s; import "syscall"; func f() { syscall.ForkExec("x", nil, nil) }`,
		"os.StartProcess":  `package s; import "os"; func f() { os.StartProcess("x", nil, nil) }`,
		"AgentCommand":     `package s; import "llm-proxy/internal/platform/process"; func f() { process.AgentCommand(nil, "ls") }`,
	}
	for name, src := range cases {
		uses, err := spawnUses("x.go", src)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		n := 0
		for _, u := range uses {
			n += len(u.spawns)
		}
		if n != 1 {
			t.Errorf("%s: want one spawn, got %d", name, n)
		}
	}
	fine := `package s; import "os/exec"; func f(c *exec.Cmd) error { return c.Run() }`
	if uses, err := spawnUses("x.go", fine); err != nil || len(uses["f"].spawns) != 0 {
		t.Errorf("running a command someone else built is not a new spawn site: %v %v", uses, err)
	}
	if uses, _ := spawnUses("x.go", `package s; func g(p interface{ Wrap() }) { p.Wrap() }`); !uses["g"].wraps {
		t.Error("a call to Wrap must be seen")
	}
}

// The privileged helper's pieces are importable only where ADR-0034 says (design section 10, test 1): the grant store (which mints) and the
// port are the composition root's; the client is the root's and the port's; the fake helper is for tests; the catalogue is read by the
// helper packages, the root and the route handler that decodes a privileged route's body. Nothing else in the service imports them, so no
// tool, assistant or automation code can reach the helper or mint a grant. Stacks cannot import any of them: they are in another module.
const privhelperPath = module + "/internal/platform/privhelper"

var privhelperImporters = map[string][]string{
	privhelperPath + "/grant":      {appPath},
	privhelperPath + "/port":       {appPath},
	privhelperPath + "/client":     {appPath, privhelperPath + "/port"},
	privhelperPath + "/helpertest": {},
	privhelperPath + "/helper":     {helperCmdPath},
	privhelperPath + "/catalog": {appPath, module + "/internal/transport/http", helperCmdPath,
		privhelperPath + "/client", privhelperPath + "/port", privhelperPath + "/helpertest", privhelperPath + "/helper"},
}

// The helper binary and its package run with privilege, so they import only an allowlist (design section 5, fitness test 2): the file jail,
// the program runner, the catalogue, the wire protocol and the helper package itself, plus the standard library and the
// kernel interface package. Anything else is a way for unreviewed code to run as the helper.
const helperCmdPath = module + "/cmd/llm-proxy-helper"

var helperAllowed = []string{
	module + "/internal/platform/isolation",
	module + "/internal/platform/progexec",
	privhelperPath + "/catalog",
	privhelperPath + "/wire",
	privhelperPath + "/helper",
	// capture is the packet reader the capture_window action runs (the helper binary under the file jail): it imports only the standard
	// library, the wire layout and x/sys/unix, and reads no frame (plan network-capture-window-plan, Constitution I.11).
	privhelperPath + "/capture",
	"golang.org/x/sys/unix",
}

// helperImportViolations lists the imports of the helper binary and its package that are neither standard library nor allowed.
func helperImportViolations(p pkg) []string {
	if p.ImportPath != helperCmdPath && p.ImportPath != privhelperPath+"/helper" {
		return nil
	}
	var bad []string
	for _, imp := range p.Imports {
		if !isStdlib(imp) && !slices.Contains(helperAllowed, imp) {
			bad = append(bad, p.ImportPath+" imports "+imp+", which is not on the privileged helper's allowlist")
		}
	}
	return bad
}

// privhelperImportViolations lists the production imports of a privileged-helper package that are not allowed. Test files are not
// production code: a test may build a store or a fake to exercise the code it is testing.
func privhelperImportViolations(p pkg) []string {
	var bad []string
	for _, imp := range p.Imports {
		allowed, guarded := privhelperImporters[imp]
		if guarded && !slices.Contains(allowed, p.ImportPath) {
			bad = append(bad, p.ImportPath+" imports "+imp+"; only "+strings.Join(allowed, ", ")+" may")
		}
	}
	return bad
}

func TestOnlyTheCompositionRootReachesTheHelperOrMintsAGrant(t *testing.T) {
	seen := 0
	for _, p := range listIn(t, backendDir(t)) {
		if strings.HasPrefix(p.ImportPath, privhelperPath+"/") {
			seen++
		}
		for _, v := range append(privhelperImportViolations(p), helperImportViolations(p)...) {
			t.Error(v)
		}
	}
	if seen < len(privhelperImporters) {
		t.Fatalf("found %d privileged-helper packages, want %d: the test would pass over nothing", seen, len(privhelperImporters))
	}
}

func TestThePrivilegedHelperRulesRefuseABreach(t *testing.T) {
	cases := map[string]pkg{
		"a tool mints a grant":           {ImportPath: module + "/internal/core/tools", Imports: []string{privhelperPath + "/grant"}},
		"the assistant calls the client": {ImportPath: module + "/internal/core/assistant", Imports: []string{privhelperPath + "/client"}},
		"a handler uses the port":        {ImportPath: module + "/internal/transport/http/handlers", Imports: []string{privhelperPath + "/port"}},
		"production uses the fake":       {ImportPath: module + "/internal/app", Imports: []string{privhelperPath + "/helpertest"}},
		"netaudit reads the catalogue":   {ImportPath: module + "/internal/core/netaudit", Imports: []string{privhelperPath + "/catalog"}},
	}
	for name, p := range cases {
		if len(privhelperImportViolations(p)) == 0 {
			t.Errorf("%s: not caught", name)
		}
	}
	fine := []pkg{
		{ImportPath: appPath, Imports: []string{privhelperPath + "/grant", privhelperPath + "/port", privhelperPath + "/client", privhelperPath + "/catalog"}},
		{ImportPath: module + "/internal/transport/http", Imports: []string{privhelperPath + "/catalog"}},
		{ImportPath: privhelperPath + "/port", Imports: []string{privhelperPath + "/client", privhelperPath + "/catalog"}},
		{ImportPath: module + "/internal/core/tools", Imports: []string{"context"}, TestImport: []string{privhelperPath + "/helpertest"}},
	}
	for _, p := range fine {
		if v := privhelperImportViolations(p); len(v) != 0 {
			t.Errorf("%s: wrongly refused: %v", p.ImportPath, v)
		}
	}
}

func TestTheHelperAllowlistRefusesABreach(t *testing.T) {
	bad := pkg{ImportPath: helperCmdPath, Imports: []string{"os", "llm-proxy/internal/platform/sandbox", "github.com/x/y"}}
	if len(helperImportViolations(bad)) != 2 {
		t.Errorf("both a service package and a third-party package must be refused: %v", helperImportViolations(bad))
	}
	fine := pkg{ImportPath: privhelperPath + "/helper", Imports: []string{"net", "golang.org/x/sys/unix", privhelperPath + "/wire", module + "/internal/platform/isolation"}}
	if v := helperImportViolations(fine); len(v) != 0 {
		t.Errorf("allowed imports refused: %v", v)
	}
	if v := helperImportViolations(pkg{ImportPath: appPath, Imports: []string{"github.com/x/y"}}); len(v) != 0 {
		t.Errorf("the allowlist is for the helper only: %v", v)
	}
}

// routerAccess are the packages that reach the router over SSH or hold what was read from it (Constitution I.15). An agent's tools, its loop
// and its prompts live in other packages: if one of them imported these, a model could name the router, the user or the key, or read the
// history. The nightly read is made by the report's own code through netaudit.RouterConnSource, wired in internal/app.
var routerAccess = []string{
	module + "/internal/platform/routerssh",
	module + "/internal/core/netaudit/routerconn",
	module + "/internal/core/netaudit/routerhistory",
}

// routerAccessAllowed are the only packages that may import them: themselves, the netaudit core that defines the source, the handlers
// that serve the operator's step-up routes, and the composition root.
var routerAccessAllowed = []string{
	module + "/internal/platform/routerssh",
	module + "/internal/core/netaudit",
	module + "/internal/core/netaudit/routerconn",
	module + "/internal/core/netaudit/routerhistory",
	module + "/internal/transport/http/handlers",
	appPath,
}

// routerAccessViolations lists the imports of the router packages by a package that is not allowed to have them.
func routerAccessViolations(p pkg) []string {
	if slices.Contains(routerAccessAllowed, p.ImportPath) || strings.HasSuffix(p.ImportPath, "_test") {
		return nil
	}
	var bad []string
	for _, imp := range p.allImports() {
		if slices.Contains(routerAccess, imp) {
			bad = append(bad, p.ImportPath+" imports "+imp)
		}
	}
	return bad
}

func TestOnlyTheServiceCodeReachesTheRouter(t *testing.T) {
	var bad []string
	for _, p := range goList(t) {
		bad = append(bad, routerAccessViolations(p)...)
	}
	if len(bad) > 0 {
		t.Fatalf("a package outside the service's own code reaches the router (Constitution I.15):\n%s", strings.Join(bad, "\n"))
	}
}

func TestTheRouterRuleRefusesABreach(t *testing.T) {
	agent := pkg{ImportPath: module + "/internal/core/tools", Imports: []string{module + "/internal/core/netaudit/routerconn"}}
	if got := routerAccessViolations(agent); len(got) != 1 {
		t.Fatalf("a tool package importing routerconn must be refused, got %v", got)
	}
	if got := routerAccessViolations(pkg{ImportPath: appPath, Imports: []string{module + "/internal/core/netaudit/routerconn"}}); len(got) != 0 {
		t.Fatalf("the composition root may wire the router, got %v", got)
	}
}
