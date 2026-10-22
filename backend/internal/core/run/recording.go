// Package run holds what every kind of run (a chat turn, an automation) sets up the same way around the agent.
package run

import (
	"path/filepath"

	"llm-proxy/internal/core/eventbus"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/rundir"
)

// runsDirName is the folder under the data root that holds run recordings.
const runsDirName = "runs"

// RecordingSpec names one run's recording: runs/<workspace>/<model>/<task>/<timestamp_id>.
type RecordingSpec struct {
	Enabled     bool   // run logging is on
	Root        string // the data root
	WorkspaceID string
	Task        string // the automation, or the conversation for a chat
	Model       string
	RunID       string
	Log         logging.Logger // nil logs nothing
}

// Recording is a run's open recording. Every field is empty when the run is not recorded; Events must be closed when
// the run ends.
type Recording struct {
	Dir           *rundir.RunDir
	Events        *eventbus.Sink
	ClientRecords bool // the LLM client writes the run's requests into Dir
}

// OpenRecording makes the run's folder and event file and points a recording client at the folder. A run that cannot
// be recorded still runs: a failure is logged and leaves the missing part empty.
func OpenRecording(client proxy.Client, spec RecordingSpec) Recording {
	if !spec.Enabled || spec.Root == "" || spec.Model == "" {
		return Recording{}
	}
	if spec.Log == nil {
		spec.Log = logging.NewNopLogger()
	}
	dir, err := rundir.NewRunDir(filepath.Join(spec.Root, runsDirName), spec.WorkspaceID, spec.Task, spec.Model)
	if err != nil {
		spec.Log.Warn("failed to create run dir, continuing without per-run output", "error", err)
		return Recording{}
	}
	rec := Recording{Dir: dir, ClientRecords: attachClient(client, spec.RunID, dir.Root)}
	if rec.Events, err = eventbus.NewSink(dir.EventsPath()); err != nil {
		spec.Log.Warn("failed to create event sink, continuing without", "error", err)
		rec.Events = nil
	}
	return rec
}

// attachClient points a recording client at the run folder and reports whether the client records.
func attachClient(client proxy.Client, runID, dir string) bool {
	if rcl, ok := client.(interface{ SetDirForRun(string, string) }); ok {
		rcl.SetDirForRun(runID, dir)
		return true
	}
	if rcl, ok := client.(interface{ SetDir(string) }); ok {
		rcl.SetDir(dir)
		return true
	}
	return false
}
