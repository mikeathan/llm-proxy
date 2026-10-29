package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"llm-proxy/internal/platform/process"
	"llm-proxy/internal/platform/storage"
)

// serverMarkerFile is written into the data folder while a server runs on it. `llm-proxy auth …` reads it to answer "does a
// live server read this folder?" without any network call, so a setup code written to the wrong folder is reported at once.
const serverMarkerFile = "server.json"

// ServerMarker says which process serves a data folder.
type ServerMarker struct {
	PID       int       `json:"pid"`
	Bind      string    `json:"bind"`
	StartedAt time.Time `json:"started_at"`
}

func ServerMarkerPath(rootDir string) string { return filepath.Join(rootDir, serverMarkerFile) }

// WriteServerMarker records this process as the server of rootDir (0600, atomically).
func WriteServerMarker(rootDir, bind string) error {
	data, err := json.Marshal(ServerMarker{PID: os.Getpid(), Bind: bind, StartedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	return storage.WriteAtomic(ServerMarkerPath(rootDir), "server-*.json.tmp", data, storage.ClassData)
}

// RemoveServerMarker deletes the marker when the server stops. A server that crashes leaves it; Alive tells that apart.
func RemoveServerMarker(rootDir string) { _ = os.Remove(ServerMarkerPath(rootDir)) }

// ReadServerMarker returns the marker of rootDir; a missing or damaged one reads as absent.
func ReadServerMarker(rootDir string) (ServerMarker, bool) {
	data, err := os.ReadFile(ServerMarkerPath(rootDir))
	if err != nil {
		return ServerMarker{}, false
	}
	var m ServerMarker
	if json.Unmarshal(data, &m) != nil || m.PID <= 0 {
		return ServerMarker{}, false
	}
	return m, true
}

// Alive reports whether the marked process is still running (a marker left by a crash is not a server).
func (m ServerMarker) Alive() bool { return process.Alive(m.PID) }
