package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_RoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	s := OpenStore(path)
	if err := s.Update(func(r *Record) error {
		r.Passphrase = "hash"
		r.Sessions = append(r.Sessions, Session{ID: "abc"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("the auth file is readable by others: %v", fi.Mode().Perm())
	}
	reopened := OpenStore(path)
	var got Record
	if err := reopened.View(func(r Record) { got = r }); err != nil || got.Passphrase != "hash" || len(got.Sessions) != 1 {
		t.Errorf("reopened: %+v, %v", got, err)
	}
}

func TestStore_ReadsTheFileAgainWhenAnotherProcessChangesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	s := OpenStore(path)
	if err := s.Update(func(r *Record) error { r.Passphrase = "one"; return nil }); err != nil {
		t.Fatal(err)
	}
	// The command line (`llm-proxy auth reset`) writes through its own store while the server runs.
	cli := OpenStore(path)
	if err := cli.Update(func(r *Record) error { r.Passphrase = ""; return nil }); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(path, future, future) // a coarse clock must not hide the change
	var got string
	_ = s.View(func(r Record) { got = r.Passphrase })
	if got != "" {
		t.Errorf("the server still sees %q after the CLI cleared it", got)
	}
}

func TestStore_AMissingFileIsAnEmptyRecord(t *testing.T) {
	var got Record
	if err := OpenStore(filepath.Join(t.TempDir(), "none.json")).View(func(r Record) { got = r }); err != nil || got.Passphrase != "" {
		t.Errorf("%+v, %v", got, err)
	}
}

func TestStore_AnUnreadableFileIsAnErrorNotAnEmptyRecord(t *testing.T) {
	// Failing open would turn a damaged file into "sign-in is off".
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := OpenStore(path).View(func(Record) {}); err == nil {
		t.Fatal("a corrupt file must be an error")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the damaged file must be left alone for the operator: %v", err)
	}
}

func TestStore_ViewGivesACopy(t *testing.T) {
	s := OpenStore(filepath.Join(t.TempDir(), "auth.json"))
	_ = s.Update(func(r *Record) error { r.Tokens = []Token{{ID: "t1", Name: "ci"}}; return nil })
	_ = s.View(func(r Record) { r.Tokens[0].Name = "changed" })
	var name string
	_ = s.View(func(r Record) { name = r.Tokens[0].Name })
	if name != "ci" {
		t.Errorf("a reader changed the store: %q", name)
	}
}

func TestStore_AFailedUpdateChangesNothing(t *testing.T) {
	s := OpenStore(filepath.Join(t.TempDir(), "auth.json"))
	_ = s.Update(func(r *Record) error { r.Passphrase = "keep"; return nil })
	if err := s.Update(func(r *Record) error { r.Passphrase = "lost"; return os.ErrInvalid }); err == nil {
		t.Fatal("the error was swallowed")
	}
	var got string
	_ = s.View(func(r Record) { got = r.Passphrase })
	if got != "keep" {
		t.Errorf("passphrase = %q, want keep", got)
	}
}
