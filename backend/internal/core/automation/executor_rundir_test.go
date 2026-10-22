package automation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// fakeClient is a proxy.Client that records nothing: only its type is looked at.
type fakeClient struct{ proxy.Client }

// runDirSvc is the service stub with run logging and a root directory the test chooses.
type runDirSvc struct {
	mockSvc
	root    string
	logging bool
}

func (s *runDirSvc) RootDir() string         { return s.root }
func (s *runDirSvc) RunLoggingEnabled() bool { return s.logging }

type dirForRunClient struct {
	fakeClient
	runID, dir string
}

func (c *dirForRunClient) SetDirForRun(runID, dir string) { c.runID, c.dir = runID, dir }

func TestRecording_NeedsLoggingARootAndAModel(t *testing.T) {
	req := ExecuteRequest{WorkspaceID: "w", AutomationName: "daily", Model: "m"}
	cases := []struct {
		name string
		svc  *runDirSvc
		req  ExecuteRequest
	}{
		{"logging off", &runDirSvc{root: t.TempDir()}, req},
		{"no root", &runDirSvc{logging: true}, req},
		{"no model", &runDirSvc{root: t.TempDir(), logging: true}, ExecuteRequest{WorkspaceID: "w", AutomationName: "daily"}},
	}
	for _, tc := range cases {
		e := &LLMTaskExecutor{svc: tc.svc}
		rec := e.recording(context.Background(), &fakeClient{}, tc.req, logging.NewNopLogger())
		if rec.Dir != nil || rec.Events != nil || rec.ClientRecords {
			t.Errorf("%s: got a run dir, sink or recording", tc.name)
		}
	}
}

func TestRecording_CreatesTheFolderAndTheEventLog(t *testing.T) {
	root := t.TempDir()
	e := &LLMTaskExecutor{svc: &runDirSvc{root: root, logging: true}}
	rec := e.recording(models.WithRunID(context.Background(), "run-7"), &dirForRunClient{}, ExecuteRequest{WorkspaceID: "w", AutomationName: "daily", Model: "m"}, logging.NewNopLogger())
	dir, sink := rec.Dir, rec.Events
	if dir == nil || sink == nil {
		t.Fatal("expected a run dir and an event sink")
	}
	defer sink.Close()
	if !rec.ClientRecords {
		t.Error("a recording client must record the run")
	}
	if rel, err := filepath.Rel(filepath.Join(root, "runs"), dir.Root); err != nil || rel == "" || rel[0] == '.' {
		t.Errorf("run folder %s is not under %s/runs", dir.Root, root)
	}
	if _, err := os.Stat(dir.Root); err != nil {
		t.Errorf("run folder missing: %v", err)
	}
}
