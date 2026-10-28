package persistence

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestExpectations_NoneYetThenRoundTripPerWorkspace(t *testing.T) {
	mgr := newBaselineManager(t)
	if got, err := mgr.ReadExpectations("ws"); err != nil || len(got) != 0 {
		t.Fatalf("no file is no expectations: %v, %v", got, err)
	}
	exp := models.Expectation{Device: "10.0.0.5", Ports: []int{}, Note: "nothing should answer", SetAt: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)}
	if err := mgr.WriteExpectations("ws", []models.Expectation{exp}); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ReadExpectations("ws")
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], exp) {
		t.Errorf("got %+v, %v", got, err)
	}
	if other, _ := mgr.ReadExpectations("other-ws"); len(other) != 0 {
		t.Errorf("another workspace must not see them: %+v", other)
	}
}

func TestExpectations_ACorruptFileIsKeptAndReported(t *testing.T) {
	mgr := newBaselineManager(t)
	path := mgr.expectationsPath("ws")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ReadExpectations("ws"); err == nil {
		t.Fatal("a corrupt file must be an error")
	}
	if _, err := os.Stat(path + corruptSuffix); err != nil {
		t.Errorf("the bad file must be kept: %v", err)
	}
}

func TestExpectations_DeletingTheWorkspaceRemovesThem(t *testing.T) {
	mgr := newBaselineManager(t)
	if err := mgr.WriteExpectations("ws", []models.Expectation{{Device: "10.0.0.5"}}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteWorkspace("ws"); err != nil {
		t.Fatal(err)
	}
	if got, _ := mgr.ReadExpectations("ws"); len(got) != 0 {
		t.Errorf("expectations outlived the workspace: %+v", got)
	}
}
