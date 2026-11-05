package persistence

import "llm-proxy/models"

// knownBadFileName holds the downloaded list of malware host names, one file in the application's data folder.
const knownBadFileName = "known-bad-names.json"

// KnownBadStore keeps the downloaded list in one file.
type KnownBadStore struct {
	file rootFile[models.KnownBad]
}

func NewKnownBadStore(dir string) *KnownBadStore {
	return &KnownBadStore{file: newRootFile[models.KnownBad](dir, knownBadFileName, "known-bad names")}
}

// Read returns the stored list. A missing file is an empty list; an undecodable one is quarantined and reported as an error.
func (s *KnownBadStore) Read() (models.KnownBad, error) { return s.file.read() }

// Write replaces the stored list atomically.
func (s *KnownBadStore) Write(r models.KnownBad) error { return s.file.write(r) }

// Delete removes the file and any copy set aside as corrupt.
func (s *KnownBadStore) Delete() error { return s.file.remove() }
