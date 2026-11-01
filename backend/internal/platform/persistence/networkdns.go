package persistence

import "llm-proxy/models"

// networkDNSFileName holds the log of DNS comparisons, one file in the application's data folder.
const networkDNSFileName = "network-dns-checks.json"

// NetworkDNSStore keeps the log of DNS comparisons in one file.
type NetworkDNSStore struct {
	file rootFile[[]models.DNSComparison]
}

func NewNetworkDNSStore(dir string) *NetworkDNSStore {
	return &NetworkDNSStore{file: newRootFile[[]models.DNSComparison](dir, networkDNSFileName, "network DNS checks")}
}

// Read returns the comparisons, oldest first. A missing file is none; an undecodable one is quarantined and reported as an error.
func (s *NetworkDNSStore) Read() ([]models.DNSComparison, error) { return s.file.read() }

// Write replaces the comparisons atomically.
func (s *NetworkDNSStore) Write(items []models.DNSComparison) error { return s.file.write(items) }

// Delete removes the file and any copy set aside as corrupt (a factory reset clears it).
func (s *NetworkDNSStore) Delete() error { return s.file.remove() }
