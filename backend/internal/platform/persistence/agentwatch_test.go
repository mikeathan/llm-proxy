package persistence

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestAgentWatchStore_RoundTripAndMissingFileIsEmpty(t *testing.T) {
	s := NewAgentWatchStore(t.TempDir())
	got, err := s.Read()
	if err != nil || len(got.Events) != 0 {
		t.Fatalf("missing file = %+v, %v; want an empty record", got, err)
	}
	in := models.AgentWatch{Events: []models.AgentWatchEvent{{At: time.Unix(1, 0).UTC(), Kind: models.AgentToolRefused, Workspace: "ws", Tool: "read_file"}}}
	if err := s.Write(in); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Read(); err != nil || len(got.Events) != 1 || got.Events[0].Tool != "read_file" {
		t.Fatalf("read back %+v, %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
}
