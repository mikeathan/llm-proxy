package persistence

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestVulnIndexStore_MissingFileIsAnEmptyIndex(t *testing.T) {
	idx, err := NewVulnIndexStore(t.TempDir()).Read()
	if err != nil || len(idx.Entries) != 0 || !idx.NVD.ImportedAt.IsZero() {
		t.Errorf("idx = %+v, err = %v", idx, err)
	}
}

func TestVulnIndexStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewVulnIndexStore(filepath.Join(dir, "nested"))
	want := models.VulnIndex{
		NVD:       models.VulnSource{ImportedAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC), CVEs: 1, Entries: 1, NoConfig: 3, Years: []int{2025, 2026}},
		Entries:   map[string][]models.VulnRange{"a:openbsd:openssh": {{CVE: "CVE-2024-0001", Score: 9.8, StartIncl: "8.0", EndExcl: "9.8"}}},
		Exploited: map[string]string{"CVE-2024-0001": "2026-10-26"},
	}
	if err := store.Write(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	r := got.Entries["a:openbsd:openssh"]
	if len(r) != 1 || r[0] != want.Entries["a:openbsd:openssh"][0] || got.Exploited["CVE-2024-0001"] != "2026-10-26" || !reflect.DeepEqual(got.NVD, want.NVD) {
		t.Errorf("got = %+v", got)
	}
}

func TestVulnIndexStore_CorruptFileIsQuarantined(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, vulnIndexFileName)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewVulnIndexStore(dir).Read(); err == nil {
		t.Fatal("a corrupt file was read")
	}
	if _, err := os.Stat(path + corruptSuffix); err != nil {
		t.Errorf("the corrupt file was not set aside: %v", err)
	}
	if idx, err := NewVulnIndexStore(dir).Read(); err != nil || len(idx.Entries) != 0 {
		t.Errorf("after quarantine: idx = %+v, err = %v", idx, err)
	}
}

func TestVulnIndexStore_DeleteRemovesTheDataAndIsFineWhenThereIsNone(t *testing.T) {
	dir := t.TempDir()
	store := NewVulnIndexStore(dir)
	if err := store.Delete(); err != nil {
		t.Fatalf("nothing to delete: %v", err)
	}
	if err := store.Write(models.VulnIndex{NVD: models.VulnSource{CVEs: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path+corruptSuffix, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{store.path, store.path + corruptSuffix} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s must be gone: %v", p, err)
		}
	}
	if idx, err := store.Read(); err != nil || idx.NVD.CVEs != 0 {
		t.Errorf("after deleting, the data is empty: %+v, %v", idx, err)
	}
}
