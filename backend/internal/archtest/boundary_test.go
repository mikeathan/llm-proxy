// Package archtest holds the fitness functions of Constitution IV.6: tests that fail the build when a stack breaches its boundary.
// It has no production code.
package archtest

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
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

func listIn(t *testing.T, dir string) []pkg {
	t.Helper()
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list in %s: %v", dir, err)
	}
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
		pkgs = append(pkgs, listIn(t, d)...)
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
	cmd := exec.Command("go", "mod", "edit", "-json")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go mod edit in %s: %v", dir, err)
	}
	var m moduleFile
	if err := json.Unmarshal(out, &m); err != nil {
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
