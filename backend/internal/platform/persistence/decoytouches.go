package persistence

import "llm-proxy/models"

// decoyTouchesFileName holds who connected to a decoy service, one file in the application's data folder.
const decoyTouchesFileName = "decoys.json"

// DecoyTouchStore keeps the decoy touches in one file.
type DecoyTouchStore struct{ file rootFile[[]models.DecoyTouch] }

func NewDecoyTouchStore(dir string) *DecoyTouchStore {
	return &DecoyTouchStore{file: newRootFile[[]models.DecoyTouch](dir, decoyTouchesFileName, "decoy touches")}
}

// Read returns the stored touches. A missing file is none; an undecodable one is quarantined and reported as an error.
func (s *DecoyTouchStore) Read() ([]models.DecoyTouch, error) { return s.file.read() }

// Write replaces the stored touches atomically.
func (s *DecoyTouchStore) Write(items []models.DecoyTouch) error { return s.file.write(items) }

// Delete removes the file and any copy set aside as corrupt.
func (s *DecoyTouchStore) Delete() error { return s.file.remove() }
