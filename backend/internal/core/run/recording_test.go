package run

import (
	"os"
	"strings"
	"testing"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
)

// recordingClient is a proxy.Client that records requests per run, like the recorder.
type recordingClient struct {
	proxy.Client
	runID, dir string
}

func (c *recordingClient) SetDirForRun(runID, dir string) { c.runID, c.dir = runID, dir }

// dirClient is an older recording client that records into one folder at a time.
type dirClient struct {
	proxy.Client
	dir string
}

func (c *dirClient) SetDir(dir string) { c.dir = dir }

func TestOpenRecording(t *testing.T) {
	spec := func(root string, enabled bool) RecordingSpec {
		return RecordingSpec{Enabled: enabled, Root: root, WorkspaceID: "ws", Task: "daily", Model: "m", RunID: "r1", Log: logging.NewNopLogger()}
	}
	t.Run("off, or nothing to name the folder after: no folder", func(t *testing.T) {
		for _, s := range []RecordingSpec{spec(t.TempDir(), false), spec("", true), {Enabled: true, Root: t.TempDir(), Log: logging.NewNopLogger()}} {
			if rec := OpenRecording(&recordingClient{}, s); rec.Dir != nil || rec.Events != nil || rec.ClientRecords {
				t.Errorf("%+v: want no recording, got %+v", s, rec)
			}
		}
	})
	t.Run("on: the folder, its event file, and the client records into it", func(t *testing.T) {
		client := &recordingClient{}
		rec := OpenRecording(client, spec(t.TempDir(), true))
		if rec.Dir == nil || rec.Events == nil || !rec.ClientRecords {
			t.Fatalf("want a full recording, got %+v", rec)
		}
		defer rec.Events.Close()
		if client.runID != "r1" || client.dir != rec.Dir.Root {
			t.Errorf("client records run %q into %q, want r1 into %q", client.runID, client.dir, rec.Dir.Root)
		}
		if _, err := os.Stat(rec.Dir.Root); err != nil || !strings.Contains(rec.Dir.Root, "/runs/ws/m/daily/") {
			t.Errorf("run folder %q: %v", rec.Dir.Root, err)
		}
	})
	t.Run("a folder-only recording client records too", func(t *testing.T) {
		client := &dirClient{}
		rec := OpenRecording(client, spec(t.TempDir(), true))
		defer rec.Events.Close()
		if !rec.ClientRecords || client.dir != rec.Dir.Root {
			t.Errorf("SetDir client: dir = %q, want %q", client.dir, rec.Dir.Root)
		}
	})
	t.Run("a folder that cannot be made is logged, even without a logger", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(root+"/"+runsDirName, nil, 0o600); err != nil { // a file where the folder must go
			t.Fatal(err)
		}
		s := spec(root, true)
		s.Log = nil
		if rec := OpenRecording(&recordingClient{}, s); rec.Dir != nil {
			t.Errorf("want no recording, got %+v", rec)
		}
	})
	t.Run("a client that does not record still gets a folder", func(t *testing.T) {
		rec := OpenRecording(nil, spec(t.TempDir(), true))
		if rec.Dir == nil || rec.ClientRecords {
			t.Errorf("want a folder and no client recording, got %+v", rec)
		}
		if rec.Events != nil {
			rec.Events.Close()
		}
	})
}
