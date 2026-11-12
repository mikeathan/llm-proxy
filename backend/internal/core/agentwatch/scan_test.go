package agentwatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scanner(dirs ...WorkspaceDir) Scanner {
	return Scanner{Workspaces: func() ([]WorkspaceDir, error) { return dirs, nil }}
}

func TestScan_FindsAKeyByPlaceNeverByValue(t *testing.T) {
	dir := t.TempDir()
	token := "lp_" + strings.Repeat("b", 52)
	writeFile(t, filepath.Join(dir, "reports", "news.md"), "The key is "+token+" for later.")
	writeFile(t, filepath.Join(dir, "notes.txt"), "nothing secret")
	writeFile(t, filepath.Join(dir, ".internal", "state.json"), token) // the program's own files are not agent output
	writeFile(t, filepath.Join(dir, "node_modules", "x.js"), token)    // nor a dependency folder
	got, err := scanner(WorkspaceDir{ID: "ws", Dir: dir}).Scan(t.Context())
	want := stacksdk.SecretHit{Workspace: "ws", Where: "reports/news.md", Kind: "a script token of this program"}
	if err != nil || len(got) != 1 || got[0] != want {
		t.Fatalf("hits = %+v err %v, want [%+v]", got, err, want)
	}
	if strings.Contains(got[0].Where+got[0].Kind, token) {
		t.Fatal("the value must never leave the file")
	}
}

func TestScan_SkipsLinksAndLargeFilesAndUnplainPaths(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	token := "lp_" + strings.Repeat("c", 52)
	writeFile(t, filepath.Join(outside, "secret.txt"), token)
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Skip("no symlinks here")
	}
	writeFile(t, filepath.Join(dir, "big.log"), strings.Repeat("x", maxScanFile)+token)
	writeFile(t, filepath.Join(dir, "odd;name.txt"), token)
	got, err := scanner(WorkspaceDir{ID: "ws", Dir: dir}).Scan(t.Context())
	if err != nil || len(got) != 0 {
		t.Errorf("hits = %+v err %v; a link is not followed, a file over the size limit is not read and a path that is not plain is not reported", got, err)
	}
}

func TestScan_ReportsEachWorkspaceInOrderAndHonoursTheFileBudget(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	token := "lp_" + strings.Repeat("d", 52)
	writeFile(t, filepath.Join(a, "one.md"), token)
	writeFile(t, filepath.Join(b, "two.md"), token)
	got, _ := scanner(WorkspaceDir{ID: "a", Dir: a}, WorkspaceDir{ID: "b", Dir: b}).Scan(t.Context())
	if len(got) != 2 || got[0].Workspace != "a" || got[1].Workspace != "b" {
		t.Fatalf("hits = %+v", got)
	}
	budget := 0
	if found := secretsIn(t.Context(), a, &budget); len(found) != 0 {
		t.Errorf("an empty budget reads nothing: %+v", found)
	}
}

func TestScan_ANilListOrAFailureReadsNothingAndStatesAFixedSentence(t *testing.T) {
	if got, err := (Scanner{}).Scan(t.Context()); got != nil || err != nil {
		t.Errorf("no workspace list turns the pass off: %+v %v", got, err)
	}
	failing := Scanner{Workspaces: func() ([]WorkspaceDir, error) { return nil, os.ErrPermission }}
	if _, err := failing.Scan(t.Context()); stacksdk.CodeOf(err) != stacksdk.CodeInternal || strings.Contains(err.Error(), "permission") {
		t.Errorf("err = %v; the failure crosses as a fixed sentence", err)
	}
}

func TestScan_StopsWhenTheContextEnds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "lp_"+strings.Repeat("e", 52))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, _ := scanner(WorkspaceDir{ID: "ws", Dir: dir}).Scan(ctx); len(got) != 0 {
		t.Errorf("hits = %+v after the context ended", got)
	}
}
