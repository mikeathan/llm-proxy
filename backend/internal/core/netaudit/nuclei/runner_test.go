package nuclei

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/progexec"
)

func TestReviewed_IsAWellFormedListOfHTTPTemplates(t *testing.T) {
	if len(Reviewed) != 24 {
		t.Fatalf("%d templates, want the 24 the owner approved", len(Reviewed))
	}
	ids := map[string]bool{}
	for _, tpl := range Reviewed {
		switch {
		case ids[tpl.ID]:
			t.Errorf("%s is listed twice", tpl.ID)
		case !strings.HasPrefix(tpl.Path, "http/") || !strings.HasSuffix(tpl.Path, ".yaml") || strings.Contains(tpl.Path, ".."):
			t.Errorf("%s: path %q is not an HTTP template file", tpl.ID, tpl.Path)
		case len(tpl.SHA256) != 64 || strings.Trim(tpl.SHA256, "0123456789abcdef") != "":
			t.Errorf("%s: %q is not a SHA-256", tpl.ID, tpl.SHA256)
		case tpl.Title == "":
			t.Errorf("%s has no title of our own", tpl.ID)
		}
		ids[tpl.ID] = true
	}
}

type fakeRunner struct {
	inv progexec.Invocation
	out progexec.Output
	err error
}

func (f *fakeRunner) Run(_ context.Context, _ progexec.Program, inv progexec.Invocation) (progexec.Output, error) {
	f.inv = inv
	return f.out, f.err
}

// templates writes a reviewed list into a folder the way the download would, and returns the list.
func templates(t *testing.T, root string) []Template {
	t.Helper()
	list := []Template{writeTemplate(t, root, "http/a.yaml", "id: a\n"), writeTemplate(t, root, "http/b.yaml", "id: b\n")}
	list[0].Title, list[1].Title = "Page A", "Page B"
	return list
}

func newChecker(r Runner, root string, list []Template) *Checker {
	return &Checker{runner: r, root: root, list: list, locate: func() (string, error) { return "/usr/bin/nuclei", nil }}
}

func TestCheck_RunsOnlyVerifiedTemplatesInAPrivateFolderAndReturnsTypedHits(t *testing.T) {
	root := t.TempDir()
	list := templates(t, root)
	line := `{"template-id":"a","info":{"severity":"low"},"host":"192.168.1.20","port":"8080","matcher-status":true}` + "\n"
	run := &fakeRunner{out: progexec.Output{Stdout: []byte(line)}}
	res, err := newChecker(run, root, list).Check(context.Background(), []Target{{Addr: mustAddr("192.168.1.20"), Port: 8080}}, 2*time.Minute)
	if err != nil || len(res.Hits) != 1 || res.Hits[0].Title != "Page A" || res.Partial {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	if !slices.Contains(run.inv.Args, filepath.Join(root, "http/a.yaml")) || !slices.Contains(run.inv.Args, "-ni") {
		t.Fatalf("args = %v", run.inv.Args)
	}
	if slices.Contains(run.inv.Env, "") || len(run.inv.Env) == 0 || run.inv.Root == "" || run.inv.Root == root {
		t.Fatalf("env = %v root = %q: the program needs its own explicit environment and folder", run.inv.Env, run.inv.Root)
	}
	if !slices.Contains(run.inv.ReadPaths, root) {
		t.Fatalf("read paths = %v, want the template folder", run.inv.ReadPaths)
	}
}

func TestCheck_RefusesWhenATemplateIsNotWhatWasReviewedAndRunsNothing(t *testing.T) {
	root := t.TempDir()
	list := templates(t, root)
	if err := os.WriteFile(filepath.Join(root, "http/a.yaml"), []byte("id: a\nevil: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := &fakeRunner{}
	if _, err := newChecker(run, root, list).Check(context.Background(), []Target{{Addr: mustAddr("192.168.1.20"), Port: 80}}, time.Minute); err == nil {
		t.Fatal("a changed template must stop the run")
	}
	if len(run.inv.Args) != 0 {
		t.Fatal("nothing may be started when a template fails its checksum")
	}
}

func TestCheck_ATimedOutRunKeepsWhatItFoundAndSaysItWasCutShort(t *testing.T) {
	root := t.TempDir()
	list := templates(t, root)
	line := `{"template-id":"b","info":{"severity":"info"},"host":"192.168.1.20","port":"80","matcher-status":true}` + "\n"
	run := &fakeRunner{out: progexec.Output{Stdout: []byte(line)}, err: progexec.ErrTimedOut}
	res, err := newChecker(run, root, list).Check(context.Background(), []Target{{Addr: mustAddr("192.168.1.20"), Port: 80}}, time.Minute)
	if err != nil || !res.Partial || len(res.Hits) != 1 {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	run.err = errors.New("boom")
	if _, err := newChecker(run, root, list).Check(context.Background(), []Target{{Addr: mustAddr("192.168.1.20"), Port: 80}}, time.Minute); err == nil {
		t.Fatal("any other failure is an error")
	}
}

func TestReady_SaysWhatIsMissingInPlainWords(t *testing.T) {
	root := t.TempDir()
	list := templates(t, root)
	c := newChecker(&fakeRunner{}, root, list)
	if ok, why := c.Ready(); !ok || why != "" {
		t.Fatalf("ready = %v %q", ok, why)
	}
	c.locate = func() (string, error) {
		return "", &progexec.NotInstalledError{Program: "nuclei", Hint: "brew install nuclei"}
	}
	if ok, why := c.Ready(); ok || !strings.Contains(why, "nuclei is not installed") {
		t.Fatalf("ready = %v %q", ok, why)
	}
	c.locate = func() (string, error) { return "/usr/bin/nuclei", nil }
	if err := os.Remove(filepath.Join(root, "http/a.yaml")); err != nil {
		t.Fatal(err)
	}
	if ok, why := c.Ready(); ok || !strings.Contains(why, "templates") {
		t.Fatalf("ready = %v %q, want the missing templates named", ok, why)
	}
}

func opener(files map[string]string, status int) Opener {
	return func(_ context.Context, url string) (*http.Response, error) {
		for path, body := range files {
			if strings.HasSuffix(url, "/"+ReviewedTag+"/"+path) {
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
}

func sum(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func TestDownload_FetchesOnlyTheReviewedFilesFromTheReviewedTagAndChecksEach(t *testing.T) {
	root := t.TempDir()
	list := []Template{{ID: "a", Path: "http/a.yaml", SHA256: sum("id: a\n"), Title: "A"}, {ID: "b", Path: "http/sub/b.yaml", SHA256: sum("id: b\n"), Title: "B"}}
	err := Download(context.Background(), opener(map[string]string{"http/a.yaml": "id: a\n", "http/sub/b.yaml": "id: b\n"}, 200), root, list)
	if err != nil {
		t.Fatal(err)
	}
	if st := StatusOf(root, list); st.Present != 2 || st.Total != 2 {
		t.Fatalf("status = %+v", st)
	}
	if info, err := os.Stat(filepath.Join(root, "http/sub/b.yaml")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("stat = %v %v, want a private file", info, err)
	}
}

func TestDownload_AFileThatIsNotWhatWasReviewedIsNeverWritten(t *testing.T) {
	root := t.TempDir()
	list := []Template{{ID: "a", Path: "http/a.yaml", SHA256: sum("id: a\n"), Title: "A"}}
	err := Download(context.Background(), opener(map[string]string{"http/a.yaml": "id: a\nevil: true\n"}, 200), root, list)
	if err == nil || !strings.Contains(err.Error(), "a") {
		t.Fatalf("err = %v, want the template named", err)
	}
	if _, err := os.Stat(filepath.Join(root, "http/a.yaml")); !os.IsNotExist(err) {
		t.Fatal("a file with the wrong checksum was written")
	}
	if err := Download(context.Background(), opener(nil, 200), root, list); err == nil {
		t.Fatal("a missing file is an error")
	}
}

func TestStatusOf_CountsOnlyTemplatesThatStillMatchTheirChecksum(t *testing.T) {
	root := t.TempDir()
	list := templates(t, root)
	if err := os.WriteFile(filepath.Join(root, "http/b.yaml"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := StatusOf(root, list); st.Present != 1 || st.Total != 2 {
		t.Fatalf("status = %+v", st)
	}
}

func TestCheck_AnOutputCutAtTheSizeLimitIsSaidToBePartial(t *testing.T) {
	root := t.TempDir()
	list := templates(t, root)
	line := `{"template-id":"a","info":{"severity":"low"},"host":"192.168.1.20","port":"8080","matcher-status":true}` + "\n"
	run := &fakeRunner{out: progexec.Output{Stdout: []byte(line), Truncated: true}}
	res, err := newChecker(run, root, list).Check(context.Background(), []Target{{Addr: mustAddr("192.168.1.20"), Port: 8080}}, time.Minute)
	if err != nil || !res.Partial || len(res.Hits) != 1 {
		t.Fatalf("res = %+v err = %v: hits lost to the size limit must not read as a complete run", res, err)
	}
}
