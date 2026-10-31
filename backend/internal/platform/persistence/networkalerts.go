package persistence

import "llm-proxy/models"

// networkAlertsFileName holds the Network alert log, one file in the application's data folder.
const networkAlertsFileName = "network-alerts.json"

// NetworkAlertStore keeps the Network alert log in one file.
type NetworkAlertStore struct {
	file rootFile[[]models.NetworkAlert]
}

func NewNetworkAlertStore(dir string) *NetworkAlertStore {
	return &NetworkAlertStore{file: newRootFile[[]models.NetworkAlert](dir, networkAlertsFileName, "network alerts")}
}

// Read returns the stored alerts, oldest first. A missing file is none; an undecodable one is quarantined and reported as an error.
func (s *NetworkAlertStore) Read() ([]models.NetworkAlert, error) { return s.file.read() }

// Write replaces the stored alerts atomically.
func (s *NetworkAlertStore) Write(items []models.NetworkAlert) error { return s.file.write(items) }

// Delete removes the file and any copy set aside as corrupt.
func (s *NetworkAlertStore) Delete() error { return s.file.remove() }
