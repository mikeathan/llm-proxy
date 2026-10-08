package automation

import (
	"context"
	"slices"
	"strings"
	"testing"

	"llm-proxy/models"
)

func newJournalDispatcher(t *testing.T, journal bool) (*Dispatcher, *AutomationEntry, *scriptedExecutor) {
	t.Helper()
	exec := &scriptedExecutor{report: "found things"}
	d, entry := newNotifyDispatcher(t, exec, &fakeNotifier{}, nil)
	entry.Journal = journal
	return d, entry, exec
}

func TestDispatcher_JournalIsInjectedIntoTheTask(t *testing.T) {
	t.Run("a stored journal and the write instruction reach the run", func(t *testing.T) {
		d, entry, exec := newJournalDispatcher(t, true)
		if err := d.persistence.WriteJournal("ws", "nightly", "## Worked\n- query: llm release notes"); err != nil {
			t.Fatal(err)
		}
		if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
			t.Fatal(err)
		}
		task := exec.tasks[0]
		for _, want := range []string{"find releases", "query: llm release notes", models.ToolAutomationJournal} {
			if !strings.Contains(task, want) {
				t.Errorf("task missing %q:\n%s", want, task)
			}
		}
		if !exec.reqs[0].Journal {
			t.Error("ExecuteRequest.Journal must be set so the executor enables the tool")
		}
	})

	t.Run("a first run is told its journal is empty", func(t *testing.T) {
		d, entry, exec := newJournalDispatcher(t, true)
		if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(exec.tasks[0], "empty") || !strings.Contains(exec.tasks[0], models.ToolAutomationJournal) {
			t.Errorf("first-run task must say the journal is empty and name the tool:\n%s", exec.tasks[0])
		}
	})

	t.Run("an automation without a journal gets nothing, even if a file exists", func(t *testing.T) {
		d, entry, exec := newJournalDispatcher(t, false)
		if err := d.persistence.WriteJournal("ws", "nightly", "stale notes"); err != nil {
			t.Fatal(err)
		}
		if err := d.executeAutomation(context.Background(), entry, ""); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(exec.tasks[0], "stale notes") || strings.Contains(exec.tasks[0], models.ToolAutomationJournal) || exec.reqs[0].Journal {
			t.Errorf("journal leaked into a journal-less run:\n%s", exec.tasks[0])
		}
	})
}

func TestRunContextFor(t *testing.T) {
	req := ExecuteRequest{AutomationName: "nightly", Journal: true}
	ctx := runContextFor(context.Background(), req, "run-1")
	if !models.IsJournalRun(ctx) || !models.IsUnattendedRun(ctx) || models.GetTaskName(ctx) != "nightly" {
		t.Error("a journal run must carry the journal flag, the unattended flag and the automation name")
	}
	req.Journal = false
	if models.IsJournalRun(runContextFor(context.Background(), req, "run-2")) {
		t.Error("a run without a journal must not carry the journal flag")
	}
}

func TestAllowedToolsFor(t *testing.T) {
	cases := []struct {
		name string
		req  ExecuteRequest
		want []string
	}{
		{"no allowlist stays unrestricted", ExecuteRequest{Journal: true}, nil},
		{"an allowlist gains the journal tool when the journal is on", ExecuteRequest{AllowedTools: []string{"read_file"}, Journal: true}, []string{"read_file", models.ToolAutomationJournal}},
		{"an allowlist is untouched without a journal", ExecuteRequest{AllowedTools: []string{"read_file"}}, []string{"read_file"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := allowedToolsFor(tc.req); !slices.Equal(got, tc.want) {
				t.Errorf("allowedToolsFor = %v, want %v", got, tc.want)
			}
		})
	}
}
