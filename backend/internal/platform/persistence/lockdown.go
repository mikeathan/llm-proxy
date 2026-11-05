package persistence

import "llm-proxy/models"

// lockdownFileName holds the lock down switch, one file in the application's data folder.
const lockdownFileName = "lockdown.json"

// LockdownStore keeps the lock down switch in one file.
type LockdownStore struct {
	file rootFile[models.Lockdown]
}

func NewLockdownStore(dir string) *LockdownStore {
	return &LockdownStore{file: newRootFile[models.Lockdown](dir, lockdownFileName, "lock down")}
}

// Read returns the switch. A missing file is off; an undecodable one is quarantined and reported as an error.
func (s *LockdownStore) Read() (models.Lockdown, error) { return s.file.read() }

// Write replaces the switch atomically.
func (s *LockdownStore) Write(l models.Lockdown) error { return s.file.write(l) }

// Delete removes the file and any copy set aside as corrupt.
func (s *LockdownStore) Delete() error { return s.file.remove() }
