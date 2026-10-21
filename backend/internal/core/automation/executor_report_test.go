package automation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

type fakeReportWriter struct {
	calls              int
	workspace, auto    string
	runID, content, at string
	err                error
}

func (f *fakeReportWriter) WriteReport(workspaceID, automation, runID string, at time.Time, content string) (string, error) {
	f.calls++
	f.workspace, f.auto, f.runID, f.content, f.at = workspaceID, automation, runID, content, at.UTC().Format(time.RFC3339)
	return "reports/x/y.md", f.err
}

func reportOutcome() runOutcome {
	return runOutcome{
		req:       ExecuteRequest{WorkspaceID: "ws1", AutomationName: "daily-news"},
		resp:      &ExecuteResponse{Report: "# News\n- item"},
		runID:     "run_7",
		startTime: time.Date(2026, 10, 21, 7, 0, 0, 0, time.UTC),
	}
}

func TestSaveFinalReport_SavesTheReportOfASuccessfulRun(t *testing.T) {
	w := &fakeReportWriter{}
	path, err := saveFinalReport(w, reportOutcome(), "# News\n- item")
	if err != nil || path != "reports/x/y.md" {
		t.Fatalf("path %q, err %v", path, err)
	}
	if w.workspace != "ws1" || w.auto != "daily-news" || w.runID != "run_7" || w.at != "2026-10-21T07:00:00Z" || w.content != "# News\n- item" {
		t.Errorf("wrote %+v", w)
	}
}

// Nothing is saved when there is nothing worth keeping: no reply from the agent, or a heartbeat that found nothing.
func TestSaveFinalReport_SkipsWhatIsNotAReport(t *testing.T) {
	quiet := reportOutcome()
	quiet.resp.Report = heartbeatOKMarker
	cases := map[string]struct {
		outcome runOutcome
		report  string
	}{
		"empty reply":     {reportOutcome(), ""},
		"blank reply":     {reportOutcome(), " \n\t"},
		"quiet heartbeat": {quiet, heartbeatOKMarker},
	}
	for name, c := range cases {
		w := &fakeReportWriter{}
		path, err := saveFinalReport(w, c.outcome, c.report)
		if path != "" || err != nil || w.calls != 0 {
			t.Errorf("%s: path %q err %v calls %d", name, path, err, w.calls)
		}
	}
}

// A secret the model quoted from a page is masked before the report is written to a new file.
func TestSaveFinalReport_MasksSecrets(t *testing.T) {
	w := &fakeReportWriter{}
	secret := "sk-" + strings.Repeat("a1B2", 8)
	if _, err := saveFinalReport(w, reportOutcome(), "key is "+secret+" ok"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.content, secret) || !strings.Contains(w.content, "[REDACTED]") {
		t.Errorf("content not masked: %q", w.content)
	}
}

func TestSaveFinalReport_ReportsAWriteFailure(t *testing.T) {
	w := &fakeReportWriter{err: errors.New("disk full")}
	if _, err := saveFinalReport(w, reportOutcome(), "body"); err == nil {
		t.Fatal("want the write error")
	}
}

// reportSvc is the service stub with a real workspace store, so a finished run's report lands in a temp workspace.
type reportSvc struct {
	mockSvc
	store *persistence.WorkspaceManager
}

func (s *reportSvc) Persistence() *persistence.WorkspaceManager { return s.store }
func (s *reportSvc) Events() assistant.EventPublisher           { return noopPublisher{} }
func (s *reportSvc) ProcessLogger(string) logging.Logger        { return logging.NewNopLogger() }

func finishRun(t *testing.T, reply string, outcome runOutcome) (*persistence.WorkspaceManager, *ExecuteResponse) {
	t.Helper()
	tmp := t.TempDir()
	store := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	e := &LLMTaskExecutor{svc: &reportSvc{store: store}}
	outcome.resp = &ExecuteResponse{State: &models.AgentState{}}
	resp, err := e.handleAgentSuccess(context.Background(), outcome, reply, nil)
	if err != nil {
		t.Fatalf("handleAgentSuccess: %v", err)
	}
	return store, resp
}

// A finished run leaves its report in the workspace, and a second run of the same automation leaves a second file
// beside it instead of replacing the first.
func TestHandleAgentSuccess_KeepsEachRunsReport(t *testing.T) {
	tmp := t.TempDir()
	store := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	e := &LLMTaskExecutor{svc: &reportSvc{store: store}}
	for i, reply := range []string{"first report", "second report"} {
		outcome := reportOutcome()
		outcome.runID = fmt.Sprintf("run_%d", i)
		outcome.resp = &ExecuteResponse{State: &models.AgentState{}}
		if _, err := e.handleAgentSuccess(context.Background(), outcome, reply, nil); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := store.ListTree("ws1")
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, f := range tree.Entries {
		if f.Type == models.TreeEntryFile && strings.HasPrefix(f.Path, "reports/daily-news/") {
			found = append(found, f.Path)
		}
	}
	if len(found) != 2 {
		t.Fatalf("want two reports under reports/daily-news/, got %v (tree %+v)", found, tree.Entries)
	}
	for i, want := range []string{"first report", "second report"} {
		got, _ := store.ReadTaskFile("ws1", found[i])
		if !strings.Contains(got, want) {
			t.Errorf("%s holds %q, want %q", found[i], got, want)
		}
	}
}

func TestHandleAgentSuccess_NoReplyLeavesNoReportFile(t *testing.T) {
	store, _ := finishRun(t, "", reportOutcome())
	tree, _ := store.ListTree("ws1")
	for _, f := range tree.Entries {
		if strings.HasPrefix(f.Path, "reports/") {
			t.Errorf("unexpected %s for a run with no reply", f.Path)
		}
	}
}

// The report file holds the agent's report as the UI shows it: a reply wrapped whole in a markdown code fence is
// unwrapped, and the saved text carries no run header.
func TestHandleAgentSuccess_SavedReportIsTheUnwrappedReply(t *testing.T) {
	store, resp := finishRun(t, "```markdown\n# Title\nbody\n```", reportOutcome())
	rel, err := store.ReadTaskFile("ws1", findReport(t, store))
	if err != nil || rel != "# Title\nbody" {
		t.Fatalf("saved %q, %v", rel, err)
	}
	if !strings.Contains(resp.Output, "### Final Report") || resp.Report != "# Title\nbody" {
		t.Errorf("response changed: report %q", resp.Report)
	}
}

// An agent that ran tools but gave no summary still finishes with a message for the history, and no report file.
func TestHandleAgentSuccess_NoSummaryMessagesAndNoFile(t *testing.T) {
	tmp := t.TempDir()
	store := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	e := &LLMTaskExecutor{svc: &reportSvc{store: store}}
	outcome := reportOutcome()
	state := &models.AgentState{}
	outcome.resp = &ExecuteResponse{State: state}
	history := []proxy.Message{{Role: "system"}, {Role: "assistant", ToolCalls: []proxy.ToolCall{{}, {}}}}
	if _, err := e.handleAgentSuccess(context.Background(), outcome, "", history); err != nil {
		t.Fatal(err)
	}
	if got := state.History[0].Output; !strings.Contains(got, "Agent executed 2 tool call(s) but returned no final summary.") {
		t.Errorf("history output %q", got)
	}
	outcome.resp = &ExecuteResponse{State: state}
	if _, err := e.handleAgentSuccess(context.Background(), outcome, "", []proxy.Message{{Role: "system"}, {Role: "assistant"}}); err != nil {
		t.Fatal(err)
	}
	if got := state.History[1].Output; !strings.Contains(got, "returned an empty response.") {
		t.Errorf("history output %q", got)
	}
	if tree, _ := store.ListTree("ws1"); len(tree.Entries) != 0 {
		t.Errorf("a run with no reply left files: %+v", tree.Entries)
	}
}

// A heartbeat that found nothing is not a report: no file, so a check every half hour does not fill the folder.
func TestHandleAgentSuccess_QuietHeartbeatLeavesNoFile(t *testing.T) {
	store, _ := finishRun(t, heartbeatOKMarker, reportOutcome())
	if tree, _ := store.ListTree("ws1"); len(tree.Entries) != 0 {
		t.Errorf("quiet heartbeat left files: %+v", tree.Entries)
	}
}

// findReport returns the one saved report's path.
func findReport(t *testing.T, store *persistence.WorkspaceManager) string {
	t.Helper()
	tree, err := store.ListTree("ws1")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tree.Entries {
		if e.Type == models.TreeEntryFile && strings.HasPrefix(e.Path, "reports/") {
			return e.Path
		}
	}
	t.Fatalf("no report saved: %+v", tree.Entries)
	return ""
}
