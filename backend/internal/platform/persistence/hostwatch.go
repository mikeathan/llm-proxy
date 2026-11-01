package persistence

import "llm-proxy/models"

// hostWatchFileName holds the host watch's baseline, one file in the application's data folder.
const hostWatchFileName = "hostwatch.json"

// HostWatchStore keeps the open ports and file digests the host watch compares against.
type HostWatchStore struct{ file rootFile[models.HostWatch] }

func NewHostWatchStore(dir string) *HostWatchStore {
	return &HostWatchStore{file: newRootFile[models.HostWatch](dir, hostWatchFileName, "host watch baseline")}
}

// Read returns the baseline. A missing file is an empty one; an undecodable one is quarantined and reported as an error.
func (s *HostWatchStore) Read() (models.HostWatch, error) { return s.file.read() }

// Write replaces the baseline atomically.
func (s *HostWatchStore) Write(w models.HostWatch) error { return s.file.write(w) }

// Delete removes the file and any copy set aside as corrupt.
func (s *HostWatchStore) Delete() error { return s.file.remove() }
