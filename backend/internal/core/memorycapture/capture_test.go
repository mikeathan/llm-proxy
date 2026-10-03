package memorycapture

import (
	"context"
	"errors"
	"testing"
)

type fakeLibrary struct {
	saved   []Candidate
	outcome Outcome
	err     error
	has     map[string]bool
}

func (f *fakeLibrary) Save(_ context.Context, _ string, c Candidate) (Outcome, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.saved = append(f.saved, c)
	if f.outcome == 0 {
		return OutcomeCreated, nil
	}
	return f.outcome, nil
}

func (f *fakeLibrary) Has(_ context.Context, _, content string) bool { return f.has[content] }

func TestCapture_SavesWhatTheUserAskedAndReportsIt(t *testing.T) {
	lib := &fakeLibrary{}
	res := Capture(context.Background(), NewExtractor(nil), lib, "ws", "Remember that the port is 5433. From now on answer briefly.")
	if len(lib.saved) != 2 {
		t.Fatalf("saved %d facts, want 2", len(lib.saved))
	}
	if len(res.Saved) != 2 || res.Saved[0] != "the port is 5433" || res.Saved[1] != "answer briefly" || res.Failed != 0 {
		t.Errorf("result = %+v", res)
	}
}

func TestCapture_ReportsOnlyWhatChanged(t *testing.T) {
	lib := &fakeLibrary{outcome: OutcomeDuplicate}
	res := Capture(context.Background(), NewExtractor(nil), lib, "ws", "Remember that the port is 5433")
	if len(res.Saved) != 0 || res.Failed != 0 {
		t.Errorf("an already-saved fact must not be announced as saved: %+v", res)
	}
}

// A memory failure must never reach the run: it is counted, not returned or panicked.
func TestCapture_AStoreErrorIsCountedNotPropagated(t *testing.T) {
	lib := &fakeLibrary{err: errors.New("database is locked")}
	res := Capture(context.Background(), NewExtractor(nil), lib, "ws", "Remember that the port is 5433")
	if res.Failed != 1 || len(res.Saved) != 0 {
		t.Errorf("result = %+v, want one failure and nothing saved", res)
	}
}

func TestCapture_NilCollaboratorsAreInert(t *testing.T) {
	if res := Capture(context.Background(), nil, &fakeLibrary{}, "ws", "Remember that the port is 5433"); len(res.Saved) != 0 {
		t.Errorf("a nil extractor saved something: %+v", res)
	}
	if res := Capture(context.Background(), NewExtractor(nil), nil, "ws", "Remember that the port is 5433"); len(res.Saved) != 0 {
		t.Errorf("a nil library saved something: %+v", res)
	}
}
