package persistence

import "llm-proxy/models"

// deviceIdentityFileName holds the SSH host key and TLS certificate fingerprints each service presented, one file in the
// application's data folder; only the audit's report writes it.
const deviceIdentityFileName = "identities.json"

// DeviceIdentityStore keeps the stored identities in one file.
type DeviceIdentityStore struct {
	file rootFile[[]models.DeviceIdentity]
}

func NewDeviceIdentityStore(dir string) *DeviceIdentityStore {
	return &DeviceIdentityStore{file: newRootFile[[]models.DeviceIdentity](dir, deviceIdentityFileName, "device identities")}
}

// Read returns the stored identities. A missing file is none; an undecodable one is quarantined and reported as an error.
func (s *DeviceIdentityStore) Read() ([]models.DeviceIdentity, error) { return s.file.read() }

// Write replaces the stored identities atomically.
func (s *DeviceIdentityStore) Write(items []models.DeviceIdentity) error { return s.file.write(items) }

// Delete removes the file and any copy set aside as corrupt.
func (s *DeviceIdentityStore) Delete() error { return s.file.remove() }
