package persistence

import (
	"os"
	"path/filepath"
	"testing"
)

type jsonFileDoc struct {
	Name string `json:"name"`
}

func TestJSONFile_RoundTripAndMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "doc.json")

	got, err := readJSONFile[jsonFileDoc](path, "doc")
	if err != nil || got != (jsonFileDoc{}) {
		t.Fatalf("a missing file must read as the zero value, got %+v, %v", got, err)
	}

	if err := writeJSONFile(path, "doc-*.json.tmp", "doc", jsonFileDoc{Name: "a"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err = readJSONFile[jsonFileDoc](path, "doc")
	if err != nil || got.Name != "a" {
		t.Fatalf("read back = %+v, %v", got, err)
	}
}

func TestJSONFile_QuarantinesACorruptFileAndRemoveClearsBoth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := readJSONFile[jsonFileDoc](path, "doc"); err == nil {
		t.Fatal("an undecodable file must be an error")
	}
	if _, err := os.Stat(path + corruptSuffix); err != nil {
		t.Errorf("the bad file must be kept as *.corrupt: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the bad file must be moved away so the next write cannot overwrite it silently")
	}

	if err := removeJSONFile(path, "doc"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(path + corruptSuffix); !os.IsNotExist(err) {
		t.Error("remove must clear the quarantined copy")
	}
	if err := removeJSONFile(path, "doc"); err != nil {
		t.Errorf("removing a missing file must not fail: %v", err)
	}
}
