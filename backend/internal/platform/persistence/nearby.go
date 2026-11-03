package persistence

import "llm-proxy/models"

// nearbyFileName holds what devices said about themselves, one file in the data folder.
const nearbyFileName = "nearby.json"

// NearbyStore keeps the result of the Nearby looks in one file.
type NearbyStore struct{ file rootFile[models.NearbyState] }

func NewNearbyStore(dir string) *NearbyStore {
	return &NearbyStore{file: newRootFile[models.NearbyState](dir, nearbyFileName, "nearby devices record")}
}

// Read returns the record; a missing file is an empty record.
func (s *NearbyStore) Read() (models.NearbyState, error) { return s.file.read() }

// Write replaces the record atomically.
func (s *NearbyStore) Write(r models.NearbyState) error { return s.file.write(r) }

// Delete removes the file and any copy set aside as corrupt.
func (s *NearbyStore) Delete() error { return s.file.remove() }
