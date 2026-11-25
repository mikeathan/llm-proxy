package brief

import (
	"errors"
	"testing"
	"time"
)

// A write is begun once, records its steps, ends done or failed, and a finished write is forgotten after JobKeep.
func TestJobs_OneWriteAtATimeWithStepsAndAnEnd(t *testing.T) {
	var j Jobs
	now := time.Date(2026, 11, 25, 7, 0, 0, 0, time.UTC)
	if _, err := j.Begin(2, now); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Begin(2, now); !errors.Is(err, ErrBusy) {
		t.Fatalf("a second write of one edition is refused, got %v", err)
	}
	if _, err := j.Begin(3, now); err != nil {
		t.Fatalf("another edition may be written meanwhile: %v", err)
	}
	j.Progress(2)(Step{Stage: StageWriting, Done: 1, Total: 3, Section: "World"})
	if got := j.List(now); len(got) != 2 || got[0].Step.Section != "World" || got[0].State != JobWriting {
		t.Fatalf("list: %+v", got)
	}
	if err := j.End(2, 9, "", now); err != nil {
		t.Fatal(err)
	}
	if err := j.End(3, 0, "the model did not answer", now); err != nil {
		t.Fatal(err)
	}
	if err := j.End(3, 0, "", now); err == nil {
		t.Error("a write that ended cannot end twice")
	}
	got := j.List(now)
	if got[0].State != JobDone || got[0].IssueID != 9 || got[1].State != JobFailed || got[1].Problem == "" {
		t.Fatalf("ends: %+v", got)
	}
	j.Progress(2)(Step{Stage: StageReading})
	if j.List(now)[0].Step.Section != "World" {
		t.Error("a step after the end is not recorded")
	}
	if _, err := j.Begin(2, now); err != nil {
		t.Errorf("a finished edition may be written again: %v", err)
	}
	if got := j.List(now.Add(JobKeep + time.Minute)); len(got) != 1 || got[0].EditionID != 2 {
		t.Errorf("a write that ended long ago is forgotten, one still writing stays: %+v", got)
	}
}
