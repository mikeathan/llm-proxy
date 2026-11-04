package persistence

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestUbuntuIndexStore_AMissingFileIsAnEmptyIndex(t *testing.T) {
	idx, err := NewUbuntuIndexStore(t.TempDir()).Read()
	if err != nil || len(idx.Notices) != 0 || idx.Release != "" {
		t.Errorf("idx = %+v, err = %v", idx, err)
	}
}

func TestUbuntuIndexStore_RoundTripsTheIndexExactly(t *testing.T) {
	store := NewUbuntuIndexStore(filepath.Join(t.TempDir(), "nested"))
	want := models.UbuntuIndex{
		Release: "noble", GeneratedAt: time.Date(2026, 11, 4, 0, 46, 40, 0, time.UTC), ImportedAt: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC),
		Notices:  map[string]models.UbuntuNotice{"USN-6663-3": {Issued: "2024-05-23", Severity: "high", CVEs: []string{"CVE-2024-0001"}}},
		Packages: map[string][]models.UbuntuFix{"openssl": {{Fixed: "3.0.13-0ubuntu3.1", USN: "USN-6663-3"}}},
	}
	if err := store.Write(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, err %v", got, err)
	}
}

func TestUbuntuIndexStore_AnUndecodableFileIsSetAsideAndDeleteRemovesBoth(t *testing.T) {
	dir := t.TempDir()
	store := NewUbuntuIndexStore(dir)
	path := filepath.Join(dir, ubuntuIndexFileName)
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(); err == nil {
		t.Fatal("a broken file is an error")
	}
	if _, err := os.Stat(path + corruptSuffix); err != nil {
		t.Errorf("the broken file was not set aside: %v", err)
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + corruptSuffix); !os.IsNotExist(err) {
		t.Error("the set-aside copy was not deleted")
	}
	if err := store.Delete(); err != nil {
		t.Errorf("deleting nothing is not an error: %v", err)
	}
}
