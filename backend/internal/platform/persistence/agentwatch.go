package persistence

import "llm-proxy/models"

// agentWatchFileName holds the agent security record, one file in the application's data folder.
const agentWatchFileName = "agent-watch.json"

// AgentWatchStore keeps the agent security record in one file.
type AgentWatchStore struct {
	file rootFile[models.AgentWatch]
}

func NewAgentWatchStore(dir string) *AgentWatchStore {
	return &AgentWatchStore{file: newRootFile[models.AgentWatch](dir, agentWatchFileName, "agent security record")}
}

// Read returns the stored record. A missing file is an empty record; an undecodable one is quarantined and reported as an error.
func (s *AgentWatchStore) Read() (models.AgentWatch, error) { return s.file.read() }

// Write replaces the stored record atomically.
func (s *AgentWatchStore) Write(r models.AgentWatch) error { return s.file.write(r) }

// Delete removes the file and any copy set aside as corrupt.
func (s *AgentWatchStore) Delete() error { return s.file.remove() }
