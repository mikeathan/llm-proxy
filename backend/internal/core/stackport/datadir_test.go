package stackport

import (
	"os"
	"path/filepath"
	"testing"

	"llm-proxy-stacks/sdk"
)

func TestDataDir_ReadsAndWritesOnlyTheFilesTheManifestLists(t *testing.T) {
	dir := t.TempDir()
	d := DataDir{Dir: dir, Files: []string{"mine.json"}}
	if _, err := d.Read("mine.json"); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("a missing file: code = %q", stacksdk.CodeOf(err))
	}
	if err := d.Write("mine.json", []byte(`[1]`)); err != nil {
		t.Fatal(err)
	}
	if got, err := d.Read("mine.json"); err != nil || string(got) != `[1]` {
		t.Errorf("read = %q, %v", got, err)
	}
	for _, name := range []string{"settings.yml", "../mine.json", "sub/mine.json", `a\b`, ""} {
		if err := d.Write(name, []byte("x")); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
			t.Errorf("write %q: code = %q", name, stacksdk.CodeOf(err))
		}
		if _, err := d.Read(name); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
			t.Errorf("read %q: code = %q", name, stacksdk.CodeOf(err))
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.yml")); err == nil {
		t.Error("a file outside the list was created")
	}
}

func TestDataDir_QuarantineSetsAFileAsideAndRemoveClearsBoth(t *testing.T) {
	dir := t.TempDir()
	d := DataDir{Dir: dir, Files: []string{"mine.json"}}
	_ = d.Write("mine.json", []byte("{bad"))
	if err := d.Quarantine("mine.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mine.json.corrupt")); err != nil {
		t.Error("the file was not set aside")
	}
	if err := d.Quarantine("mine.json"); err != nil {
		t.Errorf("quarantining a file that is not there is not an error: %v", err)
	}
	_ = d.Write("mine.json", []byte("[]"))
	if err := Remove(dir, []string{"mine.json", "never-there.json"}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}
