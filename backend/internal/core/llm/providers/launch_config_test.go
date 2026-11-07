package providers

import (
	"os"
	"path/filepath"
	"testing"

	"llm-proxy/models"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A model kept in a subfolder of the model folder is found by its file name when the registry holds no path for it: the registry stores the
// file name only, so every restart re-derives the path, and joining the model folder and the name alone points at a file that is not there.
func TestNormalizeModelConfig_FindsAModelInASubfolder(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "sage-A1.5", "Sage-A1.5-Q4_K_M.gguf")
	touch(t, want)

	got := NormalizeModelConfig(dir, models.ModelConfig{Name: "sage", Provider: "local", Filename: "Sage-A1.5-Q4_K_M.gguf"})
	if got.Path != want {
		t.Errorf("path = %q, want %q", got.Path, want)
	}
}

func TestNormalizeModelConfig_ResolutionOrder(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.gguf"))
	touch(t, filepath.Join(dir, "sub", "a.gguf"))
	touch(t, filepath.Join(dir, "x", "dup.gguf"))
	touch(t, filepath.Join(dir, "y", "dup.gguf"))
	touch(t, filepath.Join(dir, "kept", "pinned.gguf"))

	cases := []struct {
		name string
		cfg  models.ModelConfig
		want string
	}{
		{"a file directly in the folder wins over a copy in a subfolder", models.ModelConfig{Filename: "a.gguf"}, filepath.Join(dir, "a.gguf")},
		{"a stored absolute path is kept as it is", models.ModelConfig{Filename: "pinned.gguf", Path: filepath.Join(dir, "kept", "pinned.gguf")}, filepath.Join(dir, "kept", "pinned.gguf")},
		{"two files of one name are not guessed between", models.ModelConfig{Filename: "dup.gguf"}, filepath.Join(dir, "dup.gguf")},
		{"a file nowhere keeps the plain join, so the error names where it was looked for", models.ModelConfig{Filename: "missing.gguf"}, filepath.Join(dir, "missing.gguf")},
	}
	for _, c := range cases {
		c.cfg.Provider = "local"
		if got := NormalizeModelConfig(dir, c.cfg).Path; got != c.want {
			t.Errorf("%s: path = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLocateModelFile(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "deep", "er", "m.gguf"))
	if got := LocateModelFile(dir, "m.gguf"); got != filepath.Join(dir, "deep", "er", "m.gguf") {
		t.Errorf("got %q", got)
	}
	for _, bad := range []struct{ dir, name string }{{"", "m.gguf"}, {dir, ""}, {dir, "../m.gguf"}, {dir, "sub/m.gguf"}, {filepath.Join(dir, "none"), "m.gguf"}} {
		if got := LocateModelFile(bad.dir, bad.name); got != "" {
			t.Errorf("LocateModelFile(%q, %q) = %q, want nothing", bad.dir, bad.name, got)
		}
	}
}

// A cloud model's identifier is not a file, so it must never send the loader walking the model folder.
func TestNormalizeModelConfig_ACloudModelIsNeverSearchedFor(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "sub", "gpt-4o"))
	got := NormalizeModelConfig(dir, models.ModelConfig{Provider: "openai", Filename: "gpt-4o"})
	if got.Path != filepath.Join(dir, "gpt-4o") {
		t.Errorf("path = %q", got.Path)
	}
}
