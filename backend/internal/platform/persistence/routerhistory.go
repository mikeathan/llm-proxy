package persistence

import "llm-proxy/models"

// routerHistoryFileName holds the router watch's history, one file in the application's data folder.
const routerHistoryFileName = "router-history.json"

// RouterHistoryStore keeps what each device connected to (companies and countries) in one file.
type RouterHistoryStore struct {
	file rootFile[models.RouterHistory]
}

func NewRouterHistoryStore(dir string) *RouterHistoryStore {
	return &RouterHistoryStore{file: newRootFile[models.RouterHistory](dir, routerHistoryFileName, "router connection history")}
}

// Read returns the stored history. A missing file is an empty record; an undecodable one is quarantined and reported as an error.
func (s *RouterHistoryStore) Read() (models.RouterHistory, error) { return s.file.read() }

// Write replaces the stored history atomically.
func (s *RouterHistoryStore) Write(h models.RouterHistory) error { return s.file.write(h) }

// Delete removes the file and any copy set aside as corrupt.
func (s *RouterHistoryStore) Delete() error { return s.file.remove() }
