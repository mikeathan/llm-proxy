package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"llm-proxy/models"
)

// The master key decrypts secrets.json, so no agent surface may name it, read it or list it, whatever the allowed roots are. It is refused by name (like
// secrets.json) so that a widened root - an approved "allow this path" or an allowed_paths entry covering the data root - does not open it.

func TestFileToolsRefuseTheMasterKeyEvenInsideAnAllowedRoot(t *testing.T) {
	root := t.TempDir()
	cfg := models.FileSystemGuardrailsConfig{Enabled: true, AllowedPaths: []string{root}}
	for _, name := range []string{models.MasterKeyFilename, models.MasterKeyHashFilename} {
		path := filepath.Join(root, name)
		if _, err := validateFilePath(path, false, cfg); err == nil || !strings.Contains(err.Error(), "path access denied") {
			t.Errorf("reading %s must be refused, got %v", name, err)
		}
		if _, err := validateFilePath(path, true, cfg); err == nil {
			t.Errorf("writing %s must be refused", name)
		}
	}
	if _, err := validateFilePath(filepath.Join(root, "notes.txt"), false, cfg); err != nil {
		t.Errorf("an ordinary file in the same root must still be allowed: %v", err)
	}
}

func TestFileToolsRefuseASymlinkToTheMasterKey(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "workspaces", "ws1")
	if err := os.MkdirAll(ws, 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(root, models.MasterKeyFilename)
	if err := os.WriteFile(key, []byte("00"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(key, filepath.Join(ws, "notes.txt")); err != nil {
		t.Skip("symlinks unavailable")
	}
	cfg := models.FileSystemGuardrailsConfig{Enabled: true, AllowedPaths: []string{ws}}
	if _, err := validateFilePath(filepath.Join(ws, "notes.txt"), false, cfg); err == nil {
		t.Error("a link that resolves to the master key must be refused")
	}
}

func TestListingHidesTheMasterKey(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{models.MasterKeyFilename, models.MasterKeyHashFilename, "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fs := NewFileSystemTools(func(context.Context) models.FileSystemGuardrailsConfig {
		return models.FileSystemGuardrailsConfig{Enabled: true, AllowedPaths: []string{root}}
	})
	got, err := fs.ListDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	shown := fmt.Sprint(got)
	if strings.Contains(shown, models.MasterKeyFilename) || !strings.Contains(shown, "notes.txt") {
		t.Errorf("the listing must hide the key and keep other files, got %q", shown)
	}
}

func TestTerminalRefusesACommandThatNamesTheMasterKey(t *testing.T) {
	cfg := models.TerminalGuardrailsConfig{Enabled: true, AllowedCommands: []string{"cat", "python3", "node", "head", "ls"}}
	jail := t.TempDir()
	for _, cmd := range []string{
		"cat master.key",
		"head -c 8 master.key.hash",
		"cat " + filepath.Join(jail, models.MasterKeyFilename),
		"python3 -c \"print(open('" + jail + "/master.key').read())\"",
		"node -e \"require('fs').readFileSync('master.key')\"",
	} {
		var cache sync.Map
		if err := ValidateTerminalCommand(cmd, cfg, nil, &cache, jail, jail); err == nil || !strings.Contains(err.Error(), "path access denied") {
			t.Errorf("%q must be refused as a blocked path, got %v", cmd, err)
		}
	}
	var cache sync.Map
	if err := ValidateTerminalCommand("cat notes.txt", cfg, nil, &cache, jail, jail); err != nil {
		t.Errorf("an ordinary command must still pass: %v", err)
	}
}

func TestTerminalOutputNeverShowsTheMasterKeyPath(t *testing.T) {
	in := "master.key\nmaster.key.hash\nnotes.txt\n"
	if got := redactBlockedPaths(in, effectiveBlockedFilenames(nil)); got != "notes.txt\n" {
		t.Errorf("got %q", got)
	}
}
