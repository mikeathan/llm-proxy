package persistence

import "llm-proxy/models"

// footprintFileName holds the latest footprint results, one file in the application's data folder.
const footprintFileName = "footprint-results.json"

// FootprintStore keeps the latest result of each footprint check in one file. It is personal data (it is about the operator's own
// domains and addresses), so a factory reset removes it.
type FootprintStore struct {
	file rootFile[[]models.FootprintResult]
}

func NewFootprintStore(dir string) *FootprintStore {
	return &FootprintStore{file: newRootFile[[]models.FootprintResult](dir, footprintFileName, "footprint results")}
}

// Read returns the stored results. A missing file is none; an undecodable one is quarantined and reported as an error.
func (s *FootprintStore) Read() ([]models.FootprintResult, error) { return s.file.read() }

// Write replaces the stored results atomically.
func (s *FootprintStore) Write(r []models.FootprintResult) error { return s.file.write(r) }

// Delete removes the file and any copy set aside as corrupt.
func (s *FootprintStore) Delete() error { return s.file.remove() }
