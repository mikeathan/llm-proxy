package persistence

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestLife_NoneYetThenRoundTripPerWorkspace(t *testing.T) {
	mgr := newBaselineManager(t)
	if got, err := mgr.ReadLife("ws"); err != nil || len(got) != 0 {
		t.Fatalf("no file is no records: %v, %v", got, err)
	}
	at := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
	rec := models.FindingLife{Key: "10.0.0.1|tcp|445|smb_v1", FirstSeen: at, OpenSince: at, LastSeen: at, Returned: 1}
	if err := mgr.WriteLife("ws", []models.FindingLife{rec}); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ReadLife("ws")
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], rec) {
		t.Errorf("got %+v, %v", got, err)
	}
	if other, _ := mgr.ReadLife("other"); len(other) != 0 {
		t.Errorf("another workspace must not see them: %+v", other)
	}
}

func TestLife_DeletingTheWorkspaceRemovesThem(t *testing.T) {
	mgr := newBaselineManager(t)
	if err := mgr.WriteLife("ws", []models.FindingLife{{Key: "k"}}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteWorkspace("ws"); err != nil {
		t.Fatal(err)
	}
	if got, _ := mgr.ReadLife("ws"); len(got) != 0 {
		t.Errorf("history outlived the workspace: %+v", got)
	}
}

// Two writers (the nightly report and a recheck) must not lose each other's change.
func TestLife_ConcurrentUpdatesDoNotLoseEachOther(t *testing.T) {
	mgr := newBaselineManager(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k%02d", i)
			if err := mgr.UpdateLife("ws", func(r []models.FindingLife) []models.FindingLife { return append(r, models.FindingLife{Key: key}) }); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if got, _ := mgr.ReadLife("ws"); len(got) != 20 {
		t.Errorf("lost updates: %d of 20 kept", len(got))
	}
}
