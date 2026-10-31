package persistence

import "llm-proxy/models"

// networkHistoryFileName is the daily counts behind the Network page's trends, one small file in the application's data
// folder: the network belongs to the host, not to a workspace.
const networkHistoryFileName = "network-history.json"

// NetworkHistoryStore keeps the network history in one file.
type NetworkHistoryStore struct {
	file rootFile[models.NetworkHistory]
}

// NewNetworkHistoryStore keeps the history in dir. Nothing is read or created until Read or Write.
func NewNetworkHistoryStore(dir string) *NetworkHistoryStore {
	return &NetworkHistoryStore{file: newRootFile[models.NetworkHistory](dir, networkHistoryFileName, "network history")}
}

// Read returns the history; a missing file is an empty history.
func (s *NetworkHistoryStore) Read() (models.NetworkHistory, error) { return s.file.read() }

// Write replaces the history atomically.
func (s *NetworkHistoryStore) Write(h models.NetworkHistory) error { return s.file.write(h) }

// Delete removes the history (a factory reset clears it).
func (s *NetworkHistoryStore) Delete() error { return s.file.remove() }
